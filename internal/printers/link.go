package printers

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shortTimeout   = 4 * time.Second
	commandTimeout = 10 * time.Second
)

type link struct {
	p         Printer
	mu        sync.Mutex
	challenge map[string]string
}

type request struct {
	method  string
	path    string
	headers map[string]string
	body    func() (io.ReadCloser, error)
	size    int64
	timeout time.Duration
	sink    io.Writer
	idle    time.Duration
}

var errStalled = errors.New("The printer stopped responding mid-transfer")

const drainedGrace = 10 * time.Minute

type stallWatch struct {
	last    atomic.Int64
	drained atomic.Bool
}

func (s *stallWatch) touch() { s.last.Store(time.Now().UnixNano()) }

func (s *stallWatch) since() time.Duration { return time.Since(time.Unix(0, s.last.Load())) }

type touchReader struct {
	io.ReadCloser
	w *stallWatch
}

func (t touchReader) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if n > 0 {
		t.w.touch()
	}
	if err == io.EOF {
		t.w.touch()
		t.w.drained.Store(true)
	}
	return n, err
}

type touchWriter struct {
	io.Writer
	w *stallWatch
}

func (t touchWriter) Write(p []byte) (int, error) {
	n, err := t.Writer.Write(p)
	if n > 0 {
		t.w.touch()
	}
	return n, err
}

func (rq request) watched(ctx context.Context) (context.Context, request, func()) {
	if rq.idle <= 0 {
		return ctx, rq, func() {}
	}
	ctx, cancel := context.WithCancelCause(ctx)
	w := &stallWatch{}
	w.touch()
	if body := rq.body; body != nil {
		rq.body = func() (io.ReadCloser, error) {
			rc, err := body()
			if err != nil {
				return nil, err
			}
			w.drained.Store(false)
			w.touch()
			return touchReader{rc, w}, nil
		}
	}
	if rq.sink != nil {
		rq.sink = touchWriter{rq.sink, w}
	}
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(min(rq.idle/4, 5*time.Second))
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				limit := rq.idle
				if w.drained.Load() {
					limit = drainedGrace
				}
				if w.since() > limit {
					cancel(errStalled)
					return
				}
			}
		}
	}()
	return ctx, rq, func() {
		close(done)
		cancel(nil)
	}
}

var challengeParam = regexp.MustCompile(`(\w+)=(?:"([^"]*)"|([^,]*))`)

func parseChallenge(header string) map[string]string {
	out := map[string]string{}
	body := strings.TrimSpace(header)
	if len(body) >= 6 && strings.EqualFold(body[:6], "digest") {
		body = strings.TrimSpace(body[6:])
	}
	for _, m := range challengeParam.FindAllStringSubmatch(body, -1) {
		v := m[2]
		if v == "" {
			v = strings.TrimSpace(m[3])
		}
		out[strings.ToLower(m[1])] = v
	}
	return out
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func digestHeader(user, pass, method, uri string, ch map[string]string) string {
	algorithm := ch["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}
	qop := strings.TrimSpace(strings.Split(ch["qop"], ",")[0])
	nc := "00000001"
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	cnonce := hex.EncodeToString(buf)

	ha1 := md5hex(user + ":" + ch["realm"] + ":" + pass)
	ha2 := md5hex(method + ":" + uri)
	var response string
	if qop != "" {
		response = md5hex(strings.Join([]string{ha1, ch["nonce"], nc, cnonce, qop, ha2}, ":"))
	} else {
		response = md5hex(ha1 + ":" + ch["nonce"] + ":" + ha2)
	}

	h := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, response="%s"`,
		user, ch["realm"], ch["nonce"], uri, algorithm, response)
	if qop != "" {
		h += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if ch["opaque"] != "" {
		h += fmt.Sprintf(`, opaque="%s"`, ch["opaque"])
	}
	return h
}

func (l *link) getChallenge(ctx context.Context, force bool) (map[string]string, error) {
	l.mu.Lock()
	cached := l.challenge
	l.mu.Unlock()
	if cached != nil && !force {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.p.Host+"/api/v1/status", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)

	header := res.Header.Get("WWW-Authenticate")
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(strings.ToLower(header), "digest") {
		return nil, nil
	}
	ch := parseChallenge(header)
	l.mu.Lock()
	l.challenge = ch
	l.mu.Unlock()
	return ch, nil
}

func (l *link) send(ctx context.Context, rq request, auth map[string]string) (int, []byte, error) {
	var body io.ReadCloser
	if rq.body != nil {
		var err error
		if body, err = rq.body(); err != nil {
			return 0, nil, err
		}
	}
	var reader io.Reader
	if body != nil {
		reader = body
	}
	req, err := http.NewRequestWithContext(ctx, rq.method, l.p.Host+rq.path, reader)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		return 0, nil, err
	}
	if body != nil {
		req.ContentLength = rq.size
	}
	for k, v := range rq.headers {
		req.Header.Set(k, v)
	}
	for k, v := range auth {
		req.Header.Set(k, v)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if rq.sink != nil && res.StatusCode >= 200 && res.StatusCode <= 299 {
		_, err = io.Copy(rq.sink, res.Body)
		return res.StatusCode, nil, err
	}
	data, err := io.ReadAll(res.Body)
	return res.StatusCode, data, err
}

func (l *link) do(ctx context.Context, rq request) (int, []byte, error) {
	if rq.timeout == 0 {
		rq.timeout = commandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, rq.timeout)
	defer cancel()
	ctx, rq, stop := rq.watched(ctx)
	defer stop()
	status, data, err := l.attempt(ctx, rq)
	if err != nil && errors.Is(context.Cause(ctx), errStalled) {
		return 0, nil, errStalled
	}
	return status, data, err
}

func (l *link) attempt(ctx context.Context, rq request) (int, []byte, error) {

	if l.p.APIKey != "" {
		return l.send(ctx, rq, map[string]string{"X-Api-Key": l.p.APIKey})
	}
	if l.p.User == "" {
		return l.send(ctx, rq, nil)
	}

	ch, err := l.getChallenge(ctx, false)
	if err != nil {
		return 0, nil, err
	}
	if ch == nil {
		return l.send(ctx, rq, nil)
	}
	status, data, err := l.send(ctx, rq, map[string]string{
		"Authorization": digestHeader(l.p.User, l.p.Password, rq.method, rq.path, ch),
	})
	if err != nil || status != http.StatusUnauthorized {
		return status, data, err
	}

	fresh, err := l.getChallenge(ctx, true)
	if err != nil || fresh == nil {
		return status, data, err
	}
	return l.send(ctx, rq, map[string]string{
		"Authorization": digestHeader(l.p.User, l.p.Password, rq.method, rq.path, fresh),
	})
}
