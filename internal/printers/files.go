package printers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

var allowedExtensions = []string{".bgcode", ".gcode", ".bgc", ".gco", ".g"}

const maxNameLength = 255

type File struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Size        *int64 `json:"size,omitempty"`
	ReadOnly    *bool  `json:"readOnly,omitempty"`
	ModifiedAt  *int64 `json:"modifiedAt,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	PrintMeta
}

type Storage struct {
	Name       string `json:"name"`
	Available  bool   `json:"available"`
	ReadOnly   bool   `json:"readOnly"`
	FreeSpace  *int64 `json:"freeSpace,omitempty"`
	TotalSpace *int64 `json:"totalSpace,omitempty"`
}

type FilesView struct {
	Online  bool     `json:"online"`
	Storage *Storage `json:"storage,omitempty"`
	Files   []File   `json:"files"`
	Error   string   `json:"error,omitempty"`
}

type rawStorage struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	FreeSpace  *int64 `json:"free_space"`
	TotalSpace *int64 `json:"total_space"`
	Available  bool   `json:"available"`
	ReadOnly   bool   `json:"read_only"`
}

type rawChild struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name"`
	Type        string         `json:"type"`
	Size        *int64         `json:"size"`
	Ro          *bool          `json:"ro"`
	ReadOnly    *bool          `json:"read_only"`
	Timestamp   *int64         `json:"m_timestamp"`
	Meta        map[string]any `json:"meta"`
	Refs        struct {
		Download string `json:"download"`
	} `json:"refs"`
}

func sanitizeFileName(raw string) string {
	base := raw
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		base = raw[i+1:]
	}
	base = strings.TrimSpace(base)
	if base == "" || len(base) > maxNameLength || strings.HasPrefix(base, ".") {
		return ""
	}
	if strings.IndexFunc(base, unicode.IsControl) >= 0 {
		return ""
	}
	lower := strings.ToLower(base)
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(lower, ext) {
			return base
		}
	}
	return ""
}

func filePath(storage, name string) string {
	return "/api/v1/files/" + storage + "/" + url.PathEscape(name)
}

func pickStorage(list []rawStorage, override string) string {
	if override != "" {
		return trimSlashes(override)
	}
	for _, s := range list {
		if s.Available && !s.ReadOnly && s.Path != "" {
			return trimSlashes(s.Path)
		}
	}
	return "usb"
}

func (l *link) storage(ctx context.Context) (string, *Storage) {
	if l.p.Storage != "" {
		return trimSlashes(l.p.Storage), nil
	}
	var raw struct {
		List []rawStorage `json:"storage_list"`
	}
	_, _ = l.getJSON(ctx, "/api/v1/storage", &raw)
	name := pickStorage(raw.List, "")
	for _, s := range raw.List {
		if s.Path != "" && trimSlashes(s.Path) == name {
			n := s.Name
			if n == "" {
				n = name
			}
			return name, &Storage{n, s.Available, s.ReadOnly, s.FreeSpace, s.TotalSpace}
		}
	}
	return name, nil
}

func (l *link) fillSizes(ctx context.Context, storage string, files []File) {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range files {
		if files[i].Size != nil && files[i].hasMeta() {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(f *File) {
			defer wg.Done()
			defer func() { <-sem }()
			var info rawChild
			if ok, err := l.getJSON(ctx, filePath(storage, f.Name), &info); err != nil || !ok {
				return
			}
			if f.Size == nil {
				f.Size = info.Size
			}
			if !f.hasMeta() {
				f.PrintMeta = parsePrintMeta(info.Meta)
			}
			if f.ReadOnly == nil {
				f.ReadOnly = firstBool(info.Ro, info.ReadOnly)
			}
			if f.ModifiedAt == nil && info.Timestamp != nil {
				ms := *info.Timestamp * 1000
				f.ModifiedAt = &ms
			}
		}(&files[i])
	}
	wg.Wait()
}

func firstBool(a, b *bool) *bool {
	if a != nil {
		return a
	}
	return b
}

func (l *link) files(ctx context.Context) FilesView {
	view := FilesView{Files: []File{}}
	if l.p.Host == "" {
		view.Error = "PrusaLink not configured"
		return view
	}

	storage, info := l.storage(ctx)
	children, err := l.children(ctx, storage)
	if err != nil {
		view.Error = err.Error()
		return view
	}

	for _, c := range children {
		if strings.EqualFold(c.Type, "folder") || sanitizeFileName(c.Name) == "" {
			continue
		}
		f := File{Name: c.Name, DisplayName: firstString(c.DisplayName, c.Name), Size: c.Size, ReadOnly: firstBool(c.Ro, c.ReadOnly), PrintMeta: parsePrintMeta(c.Meta)}
		if c.Timestamp != nil {
			ms := *c.Timestamp * 1000
			f.ModifiedAt = &ms
		}
		view.Files = append(view.Files, f)
	}
	l.fillSizes(ctx, storage, view.Files)
	view.Online = true
	view.Storage = info
	return view
}

var transferIdle = 90 * time.Second

func transferTimeout(size int64) time.Duration {
	return max(time.Hour, time.Duration(size/(4<<10))*time.Second)
}

type countReader struct {
	io.ReadCloser
	total  int64
	onRead func(int64)
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.total += int64(n)
	c.onRead(c.total)
	return n, err
}

func (l *link) upload(ctx context.Context, name string, open func() (io.ReadCloser, error), size int64, overwrite bool, onSent func(int64)) (int, error) {
	storage, _ := l.storage(ctx)
	ow := "?0"
	if overwrite {
		ow = "?1"
	}
	status, _, err := l.do(ctx, request{
		method: http.MethodPut,
		path:   filePath(storage, name),
		headers: map[string]string{
			"Accept":             "application/json",
			"Content-Type":       "application/octet-stream",
			"Print-After-Upload": "?0",
			"Overwrite":          ow,
		},
		size:    size,
		timeout: transferTimeout(size),
		idle:    transferIdle,
		body: func() (io.ReadCloser, error) {
			onSent(0)
			f, err := open()
			if err != nil {
				return nil, err
			}
			return &countReader{ReadCloser: f, onRead: onSent}, nil
		},
	})
	return status, err
}

func (l *link) command(ctx context.Context, method, name string, extra map[string]string) (int, error) {
	storage, _ := l.storage(ctx)
	headers := map[string]string{"Accept": "application/json"}
	for k, v := range extra {
		headers[k] = v
	}
	status, _, err := l.do(ctx, request{method: method, path: filePath(storage, name), headers: headers})
	return status, err
}

func upstreamMessage(status int, op string) string {
	switch status {
	case http.StatusConflict:
		switch op {
		case "upload":
			return "A file with that name is already on the printer"
		case "print":
			return "A print job is already running"
		}
		return "That file is currently printing"
	case http.StatusInsufficientStorage:
		return "No USB drive detected in the printer. Check that one is inserted and formatted as FAT32."
	case http.StatusNotFound:
		return "The printer rejected the storage path"
	case http.StatusUnauthorized:
		return "PrusaLink rejected the credentials"
	}
	return fmt.Sprintf("PrusaLink returned %d", status)
}

func unescapeHeader(raw string) string {
	if s, err := url.PathUnescape(raw); err == nil {
		return s
	}
	return ""
}

func (l *link) fileInfo(ctx context.Context, storage, name string) (rawChild, error) {
	var info rawChild
	status, data, err := l.do(ctx, request{
		method:  http.MethodGet,
		path:    filePath(storage, name),
		headers: map[string]string{"Accept": "application/json"},
		timeout: shortTimeout,
	})
	switch {
	case err != nil:
		return info, err
	case status == http.StatusNotFound:
		return info, &statusError{http.StatusNotFound, "That file is no longer on the printer"}
	case status < 200 || status > 299:
		return info, &statusError{upstreamStatus(status), upstreamMessage(status, "read")}
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, &statusError{http.StatusBadGateway, "PrusaLink returned something unexpected"}
	}
	return info, nil
}

func downloadPath(storage, name string, info rawChild) string {
	if strings.HasPrefix(info.Refs.Download, "/") {
		return info.Refs.Download
	}
	return "/" + storage + "/" + url.PathEscape(name)
}

func (l *link) download(ctx context.Context, path string, size int64, sink io.Writer) error {
	status, _, err := l.do(ctx, request{method: http.MethodGet, path: path, sink: sink, timeout: transferTimeout(size), idle: transferIdle})
	switch {
	case err != nil:
		return err
	case status < 200 || status > 299:
		return &statusError{upstreamStatus(status), upstreamMessage(status, "download")}
	}
	return nil
}

func (l *link) children(ctx context.Context, storage string) ([]rawChild, error) {
	var folder struct {
		Children []rawChild `json:"children"`
	}
	_, err := l.getJSON(ctx, "/api/v1/files/"+storage+"/", &folder)
	return folder.Children, err
}

func (l *link) printingFile(ctx context.Context) (string, string, error) {
	var job struct {
		File struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"file"`
	}
	ok, err := l.getJSON(ctx, "/api/v1/job", &job)
	if err != nil || !ok {
		return "", "", err
	}
	return job.File.Name, job.File.DisplayName, nil
}
