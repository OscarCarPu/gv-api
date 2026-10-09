package printers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"part.bgcode":       "part.bgcode",
		"a/b/../part.gcode": "part.gcode",
		`c:\x\LIGHTH~1.BGC`: "LIGHTH~1.BGC",
		"..":                "",
		".hidden.gcode":     "",
		"notes.txt":         "",
		"bad\x00.gcode":     "",
		"":                  "",
	}
	for in, want := range cases {
		assert.Equal(t, want, sanitizeFileName(in), in)
	}
}

func TestParseChallengeAndDigest(t *testing.T) {
	ch := parseChallenge(`Digest realm="Printer API", nonce="abc", algorithm=MD5`)
	assert.Equal(t, "Printer API", ch["realm"])
	assert.Equal(t, "abc", ch["nonce"])
	assert.Equal(t, "MD5", ch["algorithm"])

	h := digestHeader("maker", "pw", "GET", "/api/v1/job", ch)
	ha1 := md5hex("maker:Printer API:pw")
	ha2 := md5hex("GET:/api/v1/job")
	assert.Contains(t, h, `response="`+md5hex(ha1+":abc:"+ha2)+`"`)
	assert.NotContains(t, h, "qop")
}

func TestRecordingName(t *testing.T) {
	at := time.Date(2026, 8, 6, 14, 32, 5, 0, time.UTC)
	name := recordingName(at)
	assert.Equal(t, "2026-08-06T14-32-05.mp4", name)
	got, ok := parseRecordingName(name)
	require.True(t, ok)
	assert.True(t, at.Equal(got))

	for _, bad := range []string{"../x.mp4", "2026-08-06T14-32-05.exe", "x.mp4", "2026-13-40T99-99-99.mp4"} {
		_, ok := parseRecordingName(bad)
		assert.False(t, ok, bad)
	}
}

func TestFormatTimeLeft(t *testing.T) {
	assert.Equal(t, "<1m", formatTimeLeft(30))
	assert.Equal(t, "14m", formatTimeLeft(14*60))
	assert.Equal(t, "7h 05m", formatTimeLeft(7*3600+5*60))
}

func TestAppendChunk(t *testing.T) {
	u := newUploads(t.TempDir())

	got, done, err := u.appendChunk("upload-1", 0, 6, []byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, int64(3), got)
	assert.False(t, done)

	got, done, err = u.appendChunk("upload-1", 0, 6, []byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, int64(3), got)
	assert.False(t, done)

	_, _, err = u.appendChunk("upload-1", 5, 6, []byte("x"))
	var ce *chunkError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, http.StatusConflict, ce.status)
	assert.Equal(t, int64(3), ce.received)

	got, done, err = u.appendChunk("upload-1", 3, 6, []byte("def"))
	require.NoError(t, err)
	assert.Equal(t, int64(6), got)
	assert.True(t, done)

	data, _ := os.ReadFile(u.path("upload-1"))
	assert.Equal(t, "abcdef", string(data))
}

func TestSignedMedia(t *testing.T) {
	s := NewService(Config{SignKey: []byte("secret")})
	exp := time.Now().Add(time.Hour).Unix()
	sig := s.sign("core-one", "2026-08-06T14-32-05.mp4", exp)

	assert.True(t, s.verify("core-one", "2026-08-06T14-32-05.mp4", itoa(exp), sig))
	assert.False(t, s.verify("core-one", "2026-08-06T14-32-06.mp4", itoa(exp), sig))
	assert.False(t, s.verify("core-one", "2026-08-06T14-32-05.mp4", itoa(exp-7200), s.sign("core-one", "2026-08-06T14-32-05.mp4", exp-7200)))
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func fakePrusa(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/storage", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"storage_list":[{"name":"USB","path":"/usb/","available":true,"read_only":false,"free_space":100,"total_space":200}]}`))
	})
	mux.HandleFunc("/api/v1/files/usb/", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			if r.Method == http.MethodPut {
				body := new(bytes.Buffer)
				_, _ = body.ReadFrom(r.Body)
				calls = append(calls, "body "+body.String())
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.URL.Path == "/api/v1/files/usb/" {
			_, _ = w.Write([]byte(`{"children":[{"name":"A.BGC","display_name":"a.bgcode","type":"PRINT_FILE"},{"name":"dir","type":"FOLDER"},{"name":"notes.txt"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"size":42,"m_timestamp":10}`))
	})
	mux.HandleFunc("/api/v1/job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testRouter(t *testing.T, host string) http.Handler {
	t.Helper()
	svc := NewService(Config{
		Printers:      []Printer{{ID: "core-one", Name: "n", Host: host}},
		RecordingsDir: t.TempDir(),
		UploadsDir:    t.TempDir(),
		MaxBytes:      1 << 30,
		SignKey:       []byte("secret"),
	})
	h := NewHandler(svc)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	h.RegisterPublicRoutes(r)
	return r
}

func do(r http.Handler, method, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestFilesAndStopJob(t *testing.T) {
	srv, _ := fakePrusa(t)
	r := testRouter(t, srv.URL)

	rec := do(r, http.MethodGet, "/domotics/printers/core-one/files", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var view FilesView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	assert.True(t, view.Online)
	require.Len(t, view.Files, 1)
	assert.Equal(t, "a.bgcode", view.Files[0].DisplayName)
	assert.Equal(t, int64(42), *view.Files[0].Size)
	assert.Equal(t, int64(100), *view.Storage.FreeSpace)

	rec = do(r, http.MethodDelete, "/domotics/printers/core-one/job", "", nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = do(r, http.MethodGet, "/domotics/printers/nope/status", "", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestChunkedUploadIsForwarded(t *testing.T) {
	srv, calls := fakePrusa(t)
	r := testRouter(t, srv.URL)
	h := map[string]string{"X-File-Name": "part.bgcode", "X-Upload-Id": "upload-12345", "X-Upload-Total": "6"}

	h["X-Upload-Offset"] = "0"
	rec := do(r, http.MethodPatch, "/domotics/printers/core-one/files", "abc", h)
	require.Equal(t, http.StatusOK, rec.Code)

	h["X-Upload-Offset"] = "3"
	rec = do(r, http.MethodPatch, "/domotics/printers/core-one/files", "def", h)
	require.Equal(t, http.StatusAccepted, rec.Code)

	require.Eventually(t, func() bool {
		rec := do(r, http.MethodGet, "/domotics/printers/core-one/files/progress?u=upload-12345", "", nil)
		return strings.Contains(rec.Body.String(), `"done"`)
	}, 3*time.Second, 20*time.Millisecond)
	assert.Contains(t, *calls, "body abcdef")
}

func TestUploadRejectsBadName(t *testing.T) {
	srv, _ := fakePrusa(t)
	r := testRouter(t, srv.URL)
	rec := do(r, http.MethodPut, "/domotics/printers/core-one/files", "x", map[string]string{"X-File-Name": "evil.sh"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRecordingsAreSignedAndServed(t *testing.T) {
	srv, _ := fakePrusa(t)
	svc := NewService(Config{
		Printers:      []Printer{{ID: "core-one", Host: srv.URL}},
		RecordingsDir: t.TempDir(),
		MaxBytes:      1 << 30,
		SignKey:       []byte("secret"),
	})
	h := NewHandler(svc)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	h.RegisterPublicRoutes(r)

	dir := filepath.Join(svc.cfg.RecordingsDir, "core-one")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-08-06T14-32-05.mp4"), []byte("0123456789"), 0o644))

	rec := do(r, http.MethodGet, "/domotics/printers/core-one/recordings", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var view RecordingsView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.Recordings, 1)

	rec = do(r, http.MethodGet, view.Recordings[0].URL, "", map[string]string{"Range": "bytes=2-4"})
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "234", rec.Body.String())

	rec = do(r, http.MethodGet, "/domotics/printers/core-one/recordings/2026-08-06T14-32-05.mp4", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
