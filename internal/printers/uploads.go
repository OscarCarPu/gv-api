package printers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxUploadBytes = 512 << 20
	staleStaging   = time.Hour
	forwardingTTL  = 30 * time.Minute
	settledTTL     = 2 * time.Minute
	maxEntries     = 32
)

var uploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

type UploadState struct {
	Sent       int64  `json:"sent"`
	Total      int64  `json:"total"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

type ActiveUpload struct {
	UploadState
	UploadID string `json:"uploadId"`
	Name     string `json:"name"`
}

type entry struct {
	UploadState
	printerID string
	name      string
	updated   time.Time
}

type uploads struct {
	dir     string
	mu      sync.Mutex
	entries map[string]*entry
}

func newUploads(dir string) *uploads {
	return &uploads{dir: dir, entries: map[string]*entry{}}
}

type chunkError struct {
	msg      string
	status   int
	received int64
}

func (e *chunkError) Error() string { return e.msg }

func (u *uploads) path(id string) string { return filepath.Join(u.dir, id+".part") }

func staged(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func (u *uploads) stageWhole(id string, r io.Reader) (int64, error) {
	if err := os.MkdirAll(u.dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(u.path(id))
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(u.path(id))
	}
	return n, err
}

func (u *uploads) appendChunk(id string, offset, total int64, chunk []byte) (int64, bool, error) {
	switch {
	case total <= 0:
		return 0, false, &chunkError{msg: "Missing upload total", status: http.StatusBadRequest}
	case total > maxUploadBytes:
		return 0, false, &chunkError{msg: "File is too large", status: http.StatusRequestEntityTooLarge}
	case len(chunk) == 0:
		return 0, false, &chunkError{msg: "Empty chunk", status: http.StatusBadRequest}
	case offset+int64(len(chunk)) > total:
		return 0, false, &chunkError{msg: "Chunk runs past the declared file size", status: http.StatusBadRequest}
	}
	if err := os.MkdirAll(u.dir, 0o755); err != nil {
		return 0, false, err
	}

	path := u.path(id)
	have := staged(path)
	if offset+int64(len(chunk)) <= have {
		return have, have == total, nil
	}
	if offset != have {
		return 0, false, &chunkError{msg: "Chunk out of order", status: http.StatusConflict, received: have}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, false, err
	}
	_, err = f.Write(chunk)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, false, err
	}
	received := have + int64(len(chunk))
	return received, received == total, nil
}

func (u *uploads) sweepStaged() {
	names, _ := filepath.Glob(filepath.Join(u.dir, "*.part"))
	for _, n := range names {
		if st, err := os.Stat(n); err == nil && time.Since(st.ModTime()) > staleStaging {
			_ = os.Remove(n)
		}
	}
}

func (u *uploads) sweepLocked() {
	for id, e := range u.entries {
		ttl := settledTTL
		if e.Status == "forwarding" {
			ttl = forwardingTTL
		}
		if time.Since(e.updated) > ttl {
			delete(u.entries, id)
		}
	}
	for len(u.entries) > maxEntries {
		oldest, at := "", time.Now()
		for id, e := range u.entries {
			if e.updated.Before(at) {
				oldest, at = id, e.updated
			}
		}
		delete(u.entries, oldest)
	}
}

func (u *uploads) start(id, printerID, name string, total int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.sweepLocked()
	u.entries[id] = &entry{
		UploadState: UploadState{Total: total, Status: "forwarding"},
		printerID:   printerID, name: name, updated: time.Now(),
	}
}

func (u *uploads) setSent(id string, sent int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if e := u.entries[id]; e != nil && e.Status == "forwarding" {
		e.Sent = sent
		e.updated = time.Now()
	}
}

func (u *uploads) finish(id string, errMsg string, httpStatus int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e := u.entries[id]
	if e == nil {
		return
	}
	e.updated = time.Now()
	if errMsg == "" {
		e.Status, e.Sent = "done", e.Total
		return
	}
	e.Status, e.Error, e.HTTPStatus = "error", errMsg, httpStatus
}

func (u *uploads) get(id string) UploadState {
	u.mu.Lock()
	defer u.mu.Unlock()
	if e := u.entries[id]; e != nil {
		return e.UploadState
	}
	return UploadState{Status: "unknown"}
}

func (u *uploads) list(printerID string) []ActiveUpload {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.sweepLocked()
	out := []ActiveUpload{}
	times := map[string]time.Time{}
	for id, e := range u.entries {
		if e.printerID == printerID {
			out = append(out, ActiveUpload{e.UploadState, id, e.name})
			times[id] = e.updated
		}
	}
	sort.Slice(out, func(i, j int) bool { return times[out[i].UploadID].Before(times[out[j].UploadID]) })
	return out
}

func (s *Service) forward(un *unit, id, name string, overwrite bool) error {
	path := s.uploads.path(id)
	size := staged(path)
	if size == 0 {
		return errors.New("nothing staged")
	}
	s.uploads.start(id, un.p.ID, name, size)

	go func() {
		defer func() { _ = os.Remove(path) }()
		open := func() (io.ReadCloser, error) { return os.Open(path) }
		status, err := un.link.upload(bg, name, open, size, overwrite, func(n int64) { s.uploads.setSent(id, n) })
		switch {
		case err != nil:
			s.uploads.finish(id, err.Error(), http.StatusBadGateway)
		case status < 200 || status > 299:
			s.uploads.finish(id, upstreamMessage(status, "upload"), status)
		default:
			s.uploads.finish(id, "", 0)
		}
	}()
	return nil
}

func parseByteHeader(raw string) (int64, bool) {
	if raw == "" || len(raw) > 15 || strings.Trim(raw, "0123456789") != "" {
		return 0, false
	}
	var n int64
	for _, c := range raw {
		n = n*10 + int64(c-'0')
	}
	return n, true
}
