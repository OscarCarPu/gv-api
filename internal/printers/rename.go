package printers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func fileExtension(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(lower, ext) {
			return name[len(name)-len(ext):]
		}
	}
	return ""
}

func renameTarget(display, requested string) (string, error) {
	req := strings.TrimSpace(requested)
	if req == "" || strings.ContainsAny(req, `/\`) {
		return "", &statusError{http.StatusBadRequest, "Invalid file name"}
	}
	ext := fileExtension(display)
	switch got := fileExtension(req); {
	case got == "":
		req += ext
	case !strings.EqualFold(got, ext):
		return "", &statusError{http.StatusBadRequest, fmt.Sprintf("Keep the %s extension", ext)}
	}
	name := sanitizeFileName(req)
	if name == "" {
		return "", &statusError{http.StatusBadRequest, "Invalid file name"}
	}
	if strings.EqualFold(name, display) {
		return "", &statusError{http.StatusBadRequest, "That is already the file's name"}
	}
	return name, nil
}

func (s *Service) rename(ctx context.Context, un *unit, name, requested string) (string, string, error) {
	storage, _ := un.link.storage(ctx)
	info, err := un.link.fileInfo(ctx, storage, name)
	if err != nil {
		return "", "", err
	}
	if v := firstBool(info.Ro, info.ReadOnly); v != nil && *v {
		return "", "", &statusError{http.StatusConflict, "That file is read-only"}
	}
	children, err := un.link.children(ctx, storage)
	if err != nil {
		return "", "", err
	}
	display := info.DisplayName
	for _, c := range children {
		if display == "" && c.Name == name {
			display = c.DisplayName
		}
	}
	display = firstString(display, name)
	newName, err := renameTarget(display, requested)
	if err != nil {
		return "", "", err
	}
	for _, c := range children {
		if strings.EqualFold(c.Name, newName) || strings.EqualFold(c.DisplayName, newName) {
			return "", "", &statusError{http.StatusConflict, "A file with that name is already on the printer"}
		}
	}
	short, long, err := un.link.printingFile(ctx)
	if err != nil {
		return "", "", err
	}
	if (short != "" && strings.EqualFold(short, name)) || (long != "" && strings.EqualFold(long, display)) {
		return "", "", &statusError{http.StatusConflict, "That file is currently printing"}
	}

	var size int64
	if info.Size != nil {
		size = *info.Size
	}
	src := downloadPath(storage, name, info)
	id := newUploadID()
	s.uploads.start(id, un.p.ID, newName, display, size)
	err = s.enqueue(un, id, func() {
		path := s.uploads.path(id)
		defer func() { _ = os.Remove(path) }()
		s.uploads.finish(id, s.moveFile(un, id, name, newName, src, path, size))
	})
	return id, newName, err
}

func (s *Service) moveFile(un *unit, id, name, newName, src, path string, size int64) *statusError {
	s.uploads.setStatus(id, statusDownloading, 0)
	if err := s.fetch(un, id, src, path, size); err != nil {
		return err
	}
	staged := staged(path)
	if size > 0 && staged != size {
		return &statusError{http.StatusBadGateway, "The printer sent an incomplete copy of the file"}
	}
	s.uploads.setStatus(id, statusForwarding, staged)
	if err := s.push(un, id, newName, path, staged, false); err != nil {
		return err
	}
	status, err := un.link.command(bg, http.MethodDelete, name, map[string]string{"Force": "?0"})
	if err == nil && (status < 200 || status > 299) {
		err = &statusError{status, upstreamMessage(status, "delete")}
	}
	if err != nil {
		return &statusError{http.StatusBadGateway, fmt.Sprintf("Saved as %s, but the original could not be deleted: %s", newName, err.Error())}
	}
	return nil
}

func (s *Service) fetch(un *unit, id, src, path string, size int64) *statusError {
	if err := os.MkdirAll(s.uploads.dir, 0o755); err != nil {
		return &statusError{http.StatusInternalServerError, "Could not stage the file"}
	}
	f, err := os.Create(path)
	if err != nil {
		return &statusError{http.StatusInternalServerError, "Could not stage the file"}
	}
	err = un.link.download(bg, src, size, &countWriter{w: f, onWrite: func(n int64) { s.uploads.setSent(id, n) }})
	if cerr := f.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		if se, ok := err.(*statusError); ok {
			return se
		}
		return &statusError{http.StatusBadGateway, "Could not read the file from the printer: " + err.Error()}
	}
	return nil
}

type countWriter struct {
	w       io.Writer
	total   int64
	onWrite func(int64)
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.total += int64(n)
	c.onWrite(c.total)
	return n, err
}

func (s *Service) fileURL(printerID string, f File) string {
	exp := time.Now().Add(mediaURLTTL).Truncate(mediaURLStep).Unix()
	return fmt.Sprintf("/domotics/printers/%s/files/download?name=%s&as=%s&exp=%d&sig=%s",
		printerID, url.QueryEscape(f.Name), url.QueryEscape(f.DisplayName), exp, s.sign(printerID, "file/"+f.Name, exp))
}

type lazyHeaders struct {
	w       http.ResponseWriter
	prepare func()
	started bool
}

func (l *lazyHeaders) Write(p []byte) (int, error) {
	if !l.started {
		l.started = true
		l.prepare()
	}
	return l.w.Write(p)
}
