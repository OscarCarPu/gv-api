package printers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		_, _ = w.Write([]byte(`{"size":42,"m_timestamp":10,"refs":{"download":"/usb/A.BGC"}}`))
	})
	mux.HandleFunc("/usb/", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(strings.Repeat("x", 42)))
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
	assert.False(t, view.Files[0].hasMeta(), "Buddy reports no meta and Connect is not configured")
	assert.Contains(t, view.Files[0].DownloadURL, "/domotics/printers/core-one/files/download?name=A.BGC&as=a.bgcode&exp=")

	rec = do(r, http.MethodGet, view.Files[0].DownloadURL, "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, strings.Repeat("x", 42), rec.Body.String())
	assert.Equal(t, `attachment; filename=a.bgcode`, rec.Header().Get("Content-Disposition"))

	rec = do(r, http.MethodGet, strings.Replace(view.Files[0].DownloadURL, "sig=", "sig=0", 1), "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-08-07T10-00-00.mp4"), []byte("x"), 0o644))
	rec = do(r, http.MethodDelete, "/domotics/printers/core-one/recordings?name=2026-08-06T14-32-05.mp4&name=2026-08-07T10-00-00.mp4&name=nope.mp4", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var res struct {
		Deleted []string          `json:"deleted"`
		Failed  map[string]string `json:"failed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.ElementsMatch(t, []string{"2026-08-06T14-32-05.mp4", "2026-08-07T10-00-00.mp4"}, res.Deleted)
	assert.Contains(t, res.Failed, "nope.mp4")
	_, err := os.Stat(filepath.Join(dir, "2026-08-07T10-00-00.mp4"))
	assert.True(t, os.IsNotExist(err))
}

func TestCameraStreamNeedsSignature(t *testing.T) {
	srv, _ := fakePrusa(t)
	r := testRouter(t, srv.URL)

	rec := do(r, http.MethodGet, "/domotics/printers/core-one/camera/stream", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = do(r, http.MethodGet, "/domotics/printers/core-one/camera/url", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["url"], "/camera/stream?exp=")
}

func TestAutoRecorder(t *testing.T) {
	on := func(state string) Telemetry { return Telemetry{Online: true, State: state} }

	t.Run("starts when a print starts and stops when it ends", func(t *testing.T) {
		var a autoRecorder
		start, stop := a.observe(on("IDLE"))
		assert.False(t, start || stop)
		start, stop = a.observe(on("PRINTING"))
		assert.True(t, start)
		assert.False(t, stop)
		a.started = true
		start, stop = a.observe(on("PAUSED"))
		assert.False(t, start || stop, "a pause keeps recording")
		start, stop = a.observe(on("PRINTING"))
		assert.False(t, start || stop)
		_, stop = a.observe(on("FINISHED"))
		assert.True(t, stop)
	})

	t.Run("a print already running at boot is only a baseline", func(t *testing.T) {
		var a autoRecorder
		start, _ := a.observe(on("PRINTING"))
		assert.False(t, start)
		_, stop := a.observe(on("IDLE"))
		assert.False(t, stop, "it never started this recording")
	})

	t.Run("an offline reading changes nothing", func(t *testing.T) {
		var a autoRecorder
		a.observe(on("IDLE"))
		a.observe(on("PRINTING"))
		a.started = true
		start, stop := a.observe(Telemetry{})
		assert.False(t, start || stop)
		_, stop = a.observe(on("STOPPED"))
		assert.True(t, stop)
	})
}

type fakeConnect struct {
	logins, refreshes, jobCalls int
	filePages                   []string
	printerCalls                int
	accessSeq                   int
	validAccess                 string
	needCode                    bool
}

func (f *fakeConnect) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/o/authorize/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sessionid"); err != nil {
			http.Redirect(w, r, "/login/?next=/o/authorize/", http.StatusFound)
			return
		}
		http.Redirect(w, r, "https://connect.example/login/auth-callback?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<form><input name="x"></form><form method="post"><input type="hidden" name="csrfmiddlewaretoken" value="tok&amp;1"><input type="hidden" name="next" value="/o/authorize/"></form>`))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("csrfmiddlewaretoken") != "tok&1" || r.Form.Get("email") != "me@x" || r.Form.Get("password") != "pw" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if f.needCode {
			http.SetCookie(w, &http.Cookie{Name: "pending", Value: "p", Path: "/"})
			http.Redirect(w, r, "/login/verify/?next=/o/authorize/", http.StatusFound)
			return
		}
		f.logins++
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "s", Path: "/"})
		http.Redirect(w, r, "/passkeys/offer/", http.StatusFound)
	})
	mux.HandleFunc("/login/verify/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<form method="post"><input type="text" name="otp_token"><input type="hidden" name="csrfmiddlewaretoken" value="v1"><input type="hidden" name="next" value="/o/authorize/"></form>`))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("otp_token") != "123456" || r.Form.Get("csrfmiddlewaretoken") != "v1" {
			w.WriteHeader(http.StatusOK)
			return
		}
		f.logins++
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "s", Path: "/"})
		http.Redirect(w, r, "/o/authorize/", http.StatusFound)
	})
	mux.HandleFunc("/o/token/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "abc" || r.Form.Get("code_verifier") == "" {
				http.Error(w, "{}", http.StatusBadRequest)
				return
			}
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, "{}", http.StatusBadRequest)
				return
			}
			f.refreshes++
		}
		f.accessSeq++
		f.validAccess = fmt.Sprintf("access-%d", f.accessSeq)
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-1","expires_in":7200}`, f.validAccess)
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+f.validAccess {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/app/printers", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(`{"printers":[{"uuid":"uuid-1"}],"pager":{}}`))
		}
	})
	mux.HandleFunc("/app/printers/uuid-1", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			f.printerCalls++
			_, _ = w.Write([]byte(`{"state":"PRINTING","chamber":{"temp":43.9,"target_temp":50.0,"fan_1_rpm":0}}`))
		}
	})
	mux.HandleFunc("/app/printers/uuid-1/files", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.filePages = append(f.filePages, r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") != "0" {
			_, _ = w.Write([]byte(`{"files":[{"type":"PRINT_FILE","path":"/usb/LATE~1.BGC","meta":{"filament_type":"ASA"}}],"pager":{}}`))
			return
		}
		files := []string{`{"type":"PRINT_FILE","path":"/usb/A.BGC","meta":{"estimated_print_time":15849,"filament_type":"PLA","filament_used_g":121.74,"filament_used_mm":40817.15}}`, `{"type":"FIRMWARE","path":"/usb/FW.BBF","meta":{"filament_type":"X"}}`}
		for len(files) < connectFilesPage {
			files = append(files, `{"type":"FILE","path":"/usb/X.TMP"}`)
		}
		_, _ = fmt.Fprintf(w, `{"files":[%s],"pager":{}}`, strings.Join(files, ","))
	})
	mux.HandleFunc("/app/printers/uuid-1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.jobCalls++
		_, _ = w.Write([]byte(`{"jobs":[
			{"state":"FIN_OK","time_printing":3600.5,"file":{"name":"A.BGC","display_name":"benchy.bgcode","meta":{"filament_type":"PLA","filament_used_g":12.46}}},
			{"state":"PRINTING","time_printing":null,"file":null}],"pager":{}}`))
	})
	return mux
}

func TestHistory(t *testing.T) {
	fake := &fakeConnect{}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	router := func(user, pass string) http.Handler {
		svc := NewService(Config{
			Printers: []Printer{{ID: "core-one", ConnectUser: user, ConnectPassword: pass, ConnectBase: srv.URL, ConnectAccount: srv.URL}},
			SignKey:  []byte("secret"),
		})
		r := chi.NewRouter()
		NewHandler(svc).RegisterRoutes(r)
		return r
	}
	get := func(r http.Handler) *httptest.ResponseRecorder {
		return do(r, http.MethodGet, "/domotics/printers/core-one/history", "", nil)
	}

	t.Run("logs in, finds the printer and keeps only the useful fields", func(t *testing.T) {
		r := router("me@x", "pw")
		rec := get(r)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got []PrintHistoryEntry
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got, 2)
		assert.Equal(t, "benchy.bgcode", got[0].FileName)
		assert.Equal(t, "PLA", got[0].Material)
		assert.Equal(t, 12.5, *got[0].FilamentGrams)
		assert.Equal(t, "FIN_OK", got[0].State)
		assert.Equal(t, "PRINTING", got[1].State)
		assert.Nil(t, got[1].TimePrinting)

		require.Equal(t, http.StatusOK, get(r).Code)
		assert.Equal(t, 1, fake.logins, "the token is reused")
	})

	t.Run("a revoked access token is refreshed without logging in again", func(t *testing.T) {
		r := router("me@x", "pw")
		require.Equal(t, http.StatusOK, get(r).Code)
		logins := fake.logins
		fake.validAccess = "rotated-elsewhere"
		require.Equal(t, http.StatusOK, get(r).Code)
		assert.Equal(t, logins, fake.logins)
		assert.Equal(t, 1, fake.refreshes)
	})

	t.Run("wrong password is a 502", func(t *testing.T) {
		rec := get(router("me@x", "nope"))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Contains(t, rec.Body.String(), "email or password")
	})

	t.Run("unconfigured is 503", func(t *testing.T) {
		assert.Equal(t, http.StatusServiceUnavailable, get(router("", "")).Code)
	})

	t.Run("an emailed code is asked for once, then the saved session is enough", func(t *testing.T) {
		fake.needCode = true
		t.Cleanup(func() { fake.needCode = false })
		file := filepath.Join(t.TempDir(), "sub", "connect.json")
		p := Printer{ID: "core-one", ConnectUser: "me@x", ConnectPassword: "pw", ConnectBase: srv.URL, ConnectAccount: srv.URL, ConnectTokenFile: file}

		withFile := func(p Printer) http.Handler {
			svc := NewService(Config{Printers: []Printer{p}, SignKey: []byte("secret")})
			r := chi.NewRouter()
			NewHandler(svc).RegisterRoutes(r)
			return r
		}

		rec := get(withFile(p))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Contains(t, rec.Body.String(), "make prusa-login")

		var out bytes.Buffer
		require.Error(t, LoginConnect(context.Background(), p, strings.NewReader("000000\n"), &out))
		assert.Contains(t, out.String(), "Code: ")
		require.NoFileExists(t, file)

		require.NoError(t, LoginConnect(context.Background(), p, strings.NewReader("123456\n"), &out))
		require.FileExists(t, file)

		logins := fake.logins
		rec = get(withFile(p))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, logins, fake.logins, "the saved refresh token is used, no new login")
	})
}

func TestRenameMovesTheFile(t *testing.T) {
	srv, calls := fakePrusa(t)
	r := testRouter(t, srv.URL)

	rec := do(r, http.MethodPost, "/domotics/printers/core-one/files/rename", `{"name":"A.BGC","newName":"b"}`, nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var res struct{ UploadID, Name string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, "b.bgcode", res.Name)

	require.Eventually(t, func() bool {
		rec := do(r, http.MethodGet, "/domotics/printers/core-one/files/progress?u="+res.UploadID, "", nil)
		return strings.Contains(rec.Body.String(), `"done"`)
	}, 3*time.Second, 20*time.Millisecond)
	assert.Contains(t, *calls, "GET /usb/A.BGC")
	assert.Contains(t, *calls, "PUT /api/v1/files/usb/b.bgcode")
	assert.Contains(t, *calls, "body "+strings.Repeat("x", 42))
	assert.Contains(t, *calls, "DELETE /api/v1/files/usb/A.BGC")

	rec = do(r, http.MethodPost, "/domotics/printers/core-one/files/rename", `{"name":"A.BGC","newName":"A.bgcode"}`, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameTarget(t *testing.T) {
	cases := []struct{ display, requested, want string }{
		{"benchy.bgcode", "boat", "boat.bgcode"},
		{"benchy.bgcode", "boat.BGCODE", "boat.BGCODE"},
		{"part.gcode", " v2 part ", "v2 part.gcode"},
	}
	for _, c := range cases {
		got, err := renameTarget(c.display, c.requested)
		require.NoError(t, err, c.requested)
		assert.Equal(t, c.want, got)
	}
	for _, bad := range []string{"", "a/b", "x.gcode", "BENCHY", ".hidden"} {
		_, err := renameTarget("benchy.bgcode", bad)
		assert.Error(t, err, bad)
	}
}

func TestParsePrintMeta(t *testing.T) {
	m := parsePrintMeta(map[string]any{
		"estimated printing time (normal mode)": "1d 2h 3m",
		"filament_type":                         "PLA;PETG;PLA",
		"filament used [g]":                     "10.04, 2.5, 0.00",
	})
	assert.Equal(t, int64(93780), *m.PrintSeconds)
	assert.Equal(t, "PLA, PETG", m.Material)
	assert.Equal(t, 12.5, *m.FilamentGrams)
	assert.Nil(t, m.FilamentMeters)

	m = parsePrintMeta(map[string]any{"estimated_print_time": float64(90)})
	assert.Equal(t, int64(90), *m.PrintSeconds)

	assert.Equal(t, 5.0, *scaleUsed(ptr(10.0), doneFraction("FIN_STOPPED", ptr(50.0), nil, nil)))
	assert.Equal(t, 2.5, *scaleUsed(ptr(10.0), doneFraction("FIN_ERROR", nil, ptr(900.0), ptr(int64(3600)))))
	assert.Equal(t, 10.0, *scaleUsed(ptr(10.0), doneFraction("FIN_OK", ptr(50.0), ptr(1.0), ptr(int64(10)))))
	assert.Equal(t, 10.0, *scaleUsed(ptr(10.0), doneFraction("PRINTING", ptr(50.0), nil, nil)))
}

func ptr[T any](v T) *T { return &v }

func TestUploadsReachThePrinterOneAtATime(t *testing.T) {
	gate := make(chan struct{})
	var mu sync.Mutex
	active, peak := 0, 0
	var order []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/storage", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"storage_list":[{"path":"/usb/","available":true}]}`))
	})
	mux.HandleFunc("/api/v1/files/usb/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		active++
		peak = max(peak, active)
		order = append(order, r.URL.Path)
		mu.Unlock()
		<-gate
		mu.Lock()
		active--
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	r := testRouter(t, srv.URL)

	progress := func(id string) string {
		return do(r, http.MethodGet, "/domotics/printers/core-one/files/progress?u="+id, "", nil).Body.String()
	}
	for _, n := range []string{"one", "two"} {
		rec := do(r, http.MethodPut, "/domotics/printers/core-one/files", "data", map[string]string{"X-File-Name": n + ".gcode", "X-Upload-Id": "upload-" + n})
		require.Equal(t, http.StatusAccepted, rec.Code)
	}
	require.Eventually(t, func() bool { return strings.Contains(progress("upload-one"), `"forwarding"`) }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Contains(t, progress("upload-two"), `"queued"`)

	close(gate)
	require.Eventually(t, func() bool { return strings.Contains(progress("upload-two"), `"done"`) }, 3*time.Second, 10*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, peak)
	assert.Equal(t, []string{"/api/v1/files/usb/one.gcode", "/api/v1/files/usb/two.gcode"}, order)
}

func TestConnectFileMetaAndChamber(t *testing.T) {
	fake := &fakeConnect{}
	connect := httptest.NewServer(fake.handler())
	t.Cleanup(connect.Close)
	router := func(user string) http.Handler {
		svc := NewService(Config{
			Printers: []Printer{{ID: "core-one", ConnectUser: user, ConnectPassword: "pw", ConnectBase: connect.URL, ConnectAccount: connect.URL}},
			SignKey:  []byte("secret"),
		})
		r := chi.NewRouter()
		NewHandler(svc).RegisterRoutes(r)
		return r
	}
	r := router("me@x")

	rec := do(r, http.MethodGet, "/domotics/printers/core-one/files/meta", "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Files map[string]PrintMeta `json:"files"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	f := res.Files["A.BGC"]
	assert.Equal(t, int64(15849), *f.PrintSeconds)
	assert.Equal(t, "PLA", f.Material)
	assert.Equal(t, 121.7, *f.FilamentGrams)
	assert.Equal(t, 40.82, *f.FilamentMeters)
	assert.Equal(t, "ASA", res.Files["LATE~1.BGC"].Material)
	assert.NotContains(t, res.Files, "FW.BBF")
	assert.Equal(t, []string{"0", "100"}, fake.filePages, "a full page asks for the next one")

	for range 2 {
		rec = do(r, http.MethodGet, "/domotics/printers/core-one/chamber", "", nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"temp":43.9,"targetTemp":50}`, rec.Body.String())
	}
	assert.Equal(t, 1, fake.printerCalls, "the chamber reading is cached briefly")

	assert.Equal(t, http.StatusServiceUnavailable, do(router(""), http.MethodGet, "/domotics/printers/core-one/chamber", "", nil).Code)
}

func TestTransfersStopOnlyWhenThePrinterStalls(t *testing.T) {
	old := transferIdle
	transferIdle = 300 * time.Millisecond
	t.Cleanup(func() { transferIdle = old })

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/storage", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"storage_list":[{"path":"/usb/","available":true}]}`))
	})
	mux.HandleFunc("/usb/SLOW.GCO", func(w http.ResponseWriter, _ *http.Request) {
		for range 10 {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	})
	mux.HandleFunc("/usb/STUCK.GCO", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	l := &link{p: Printer{Host: srv.URL}}

	var got bytes.Buffer
	start := time.Now()
	require.NoError(t, l.download(context.Background(), "/usb/SLOW.GCO", 10, &got), "slow but moving is fine")
	assert.Equal(t, "xxxxxxxxxx", got.String())
	assert.Greater(t, time.Since(start), 900*time.Millisecond)

	start = time.Now()
	err := l.download(context.Background(), "/usb/STUCK.GCO", 10, io.Discard)
	assert.ErrorIs(t, err, errStalled)
	assert.Less(t, time.Since(start), 2*time.Second)
}
