package printers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	overlayPoll  = 5 * time.Second
	stopGrace    = 5 * time.Second
	stopKill     = 3 * time.Second
	startProbe   = 8 * time.Second
	stderrKeep   = 2000
	nameLayout   = "2006-01-02T15-04-05"
	defaultFont  = "/usr/share/fonts/dejavu/DejaVuSansMono-Bold.ttf"
	mediaURLTTL  = 12 * time.Hour
	mediaURLStep = time.Hour
)

var recordingNameRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})\.(mp4|jpg)$`)

type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

type Recording struct {
	Name       string `json:"name"`
	StartedAt  string `json:"startedAt"`
	EndedAt    string `json:"endedAt,omitempty"`
	DurationMs int64  `json:"durationMs"`
	SizeBytes  int64  `json:"sizeBytes"`
	Recording  bool   `json:"recording"`
	Poster     string `json:"poster,omitempty"`
	URL        string `json:"url,omitempty"`
	PosterURL  string `json:"posterUrl,omitempty"`
}

type RecordingsView struct {
	Recordings []Recording `json:"recordings"`
	UsedBytes  int64       `json:"usedBytes"`
	MaxBytes   int64       `json:"maxBytes"`
}

func recordingName(at time.Time) string {
	return at.UTC().Format(nameLayout) + ".mp4"
}

func parseRecordingName(name string) (time.Time, bool) {
	m := recordingNameRe.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation(nameLayout, m[1], time.UTC)
	return at, err == nil
}

func posterName(mp4 string) string {
	return strings.TrimSuffix(mp4, ".mp4") + ".jpg"
}

type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrKeep {
		t.buf = t.buf[len(t.buf)-stderrKeep:]
	}
	return len(p), nil
}

func (t *tail) lastLine() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

type active struct {
	name        string
	path        string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stderr      *tail
	done        chan struct{}
	stopOnce    sync.Once
	stopOverlay func()
}

type recorder struct {
	cfg    Config
	p      Printer
	link   *link
	mu     sync.Mutex
	active *active
}

func (r *recorder) dir() string { return filepath.Join(r.cfg.RecordingsDir, r.p.ID) }

func (r *recorder) liveName() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		return ""
	}
	return r.active.name
}

func (r *recorder) list(live string) []Recording {
	entries, _ := os.ReadDir(r.dir())
	posters := map[string]bool{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			posters[e.Name()] = true
		}
	}

	out := []Recording{}
	for _, e := range entries {
		started, ok := parseRecordingName(e.Name())
		if !ok || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		st, err := e.Info()
		if err != nil || !st.Mode().IsRegular() {
			continue
		}

		rec := Recording{
			Name:      e.Name(),
			StartedAt: started.Format(time.RFC3339),
			SizeBytes: st.Size(),
			Recording: e.Name() == live,
		}
		end := st.ModTime()
		if rec.Recording {
			end = time.Now()
		} else {
			rec.EndedAt = end.UTC().Format(time.RFC3339)
		}
		rec.DurationMs = max(0, end.Sub(started).Milliseconds())
		if posters[posterName(e.Name())] {
			rec.Poster = posterName(e.Name())
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

func (r *recorder) removeFiles(name string) {
	_ = os.Remove(filepath.Join(r.dir(), name))
	_ = os.Remove(filepath.Join(r.dir(), posterName(name)))
}

func (r *recorder) prune(live string) {
	list := r.list(live)
	var total int64
	for _, rec := range list {
		total += rec.SizeBytes
	}
	if total <= r.cfg.MaxBytes {
		return
	}

	var prunable []Recording
	for i := len(list) - 1; i >= 0; i-- {
		if !list[i].Recording {
			prunable = append(prunable, list[i])
		}
	}
	if len(prunable) > 0 {
		prunable = prunable[:len(prunable)-1]
	}
	for _, rec := range prunable {
		if total <= r.cfg.MaxBytes {
			return
		}
		r.removeFiles(rec.Name)
		total -= rec.SizeBytes
	}
}

func (r *recorder) freeName() (string, time.Time, error) {
	at := time.Now()
	for range 60 {
		name := recordingName(at)
		if _, err := os.Stat(filepath.Join(r.dir(), name)); err != nil {
			return name, at, nil
		}
		at = at.Add(time.Second)
	}
	return "", at, &statusError{http.StatusBadGateway, "Could not allocate a recording file name"}
}

func (r *recorder) start() (Recording, error) {
	r.mu.Lock()
	if r.active != nil {
		r.mu.Unlock()
		return Recording{}, &statusError{http.StatusConflict, "This printer is already recording"}
	}
	if err := os.MkdirAll(r.dir(), 0o755); err != nil {
		r.mu.Unlock()
		return Recording{}, &statusError{http.StatusBadGateway, "Recordings folder is not writable: " + err.Error()}
	}
	r.prune("")

	name, at, err := r.freeName()
	if err != nil {
		r.mu.Unlock()
		return Recording{}, err
	}
	path := filepath.Join(r.dir(), name)

	overlayPath := ""
	if r.cfg.Overlay {
		overlayPath = filepath.Join(os.TempDir(), "gv-recording-overlay-"+r.p.ID+".txt")
		writeOverlay(overlayPath, overlayText(nil))
	}

	args := []string{"-loglevel", "error", "-rtsp_transport", "tcp", "-i", r.p.RTSP, "-an"}
	args = append(args, r.videoArgs(overlayPath)...)
	args = append(args, "-f", "mp4",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-t", strconv.Itoa(r.cfg.MaxMinutes*60),
		"-fs", strconv.FormatInt(r.cfg.MaxBytes, 10),
		path)

	cmd := exec.Command("ffmpeg", args...)
	stderr := &tail{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		r.mu.Unlock()
		if overlayPath != "" {
			_ = os.Remove(overlayPath)
		}
		return Recording{}, &statusError{http.StatusBadGateway, err.Error()}
	}

	a := &active{name: name, path: path, cmd: cmd, stdin: stdin, stderr: stderr, done: make(chan struct{}), stopOverlay: func() {}}
	if overlayPath != "" {
		a.stopOverlay = r.pollOverlay(overlayPath)
	}
	r.active = a
	r.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		close(a.done)
		a.stopOverlay()
		r.mu.Lock()
		if r.active == a {
			r.active = nil
		}
		r.mu.Unlock()
		makePoster(path)
		r.prune(r.liveName())
	}()

	deadline := time.Now().Add(startProbe)
	for time.Now().Before(deadline) {
		select {
		case <-a.done:
			r.removeFiles(name)
			msg := a.stderr.lastLine()
			if msg == "" {
				msg = "ffmpeg stopped immediately"
			}
			return Recording{}, &statusError{http.StatusBadGateway, msg}
		default:
		}
		if st, err := os.Stat(path); err == nil && st.Size() > 0 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	var size int64
	if st, err := os.Stat(path); err == nil {
		size = st.Size()
	}
	return Recording{
		Name:       name,
		StartedAt:  at.UTC().Format(time.RFC3339),
		DurationMs: max(0, time.Since(at).Milliseconds()),
		SizeBytes:  size,
		Recording:  true,
	}, nil
}

func (r *recorder) stop() error {
	r.mu.Lock()
	a := r.active
	r.mu.Unlock()
	if a == nil {
		return &statusError{http.StatusConflict, "This printer is not recording"}
	}

	a.stopOnce.Do(func() {
		_, _ = a.stdin.Write([]byte("q"))
		_ = a.stdin.Close()
	})
	select {
	case <-a.done:
		return nil
	case <-time.After(stopGrace):
	}
	_ = a.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-a.done:
	case <-time.After(stopKill):
		_ = a.cmd.Process.Kill()
		<-a.done
	}
	return nil
}

func (r *recorder) remove(name string) error {
	if _, ok := parseRecordingName(name); !ok {
		return &statusError{http.StatusBadRequest, "Invalid recording name"}
	}
	if r.liveName() == name {
		return &statusError{http.StatusConflict, "That recording is still running. Stop it first."}
	}
	if _, err := os.Stat(filepath.Join(r.dir(), name)); err != nil {
		return &statusError{http.StatusNotFound, "Recording not found"}
	}
	r.removeFiles(name)
	return nil
}

func (r *recorder) open(name string) (*os.File, os.FileInfo, bool) {
	if _, ok := parseRecordingName(name); !ok {
		return nil, nil, false
	}
	f, err := os.Open(filepath.Join(r.dir(), name))
	if err != nil {
		return nil, nil, false
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, false
	}
	return f, st, true
}

func formatTimeLeft(seconds float64) string {
	minutes := int(seconds) / 60
	if minutes < 1 {
		return "<1m"
	}
	if h := minutes / 60; h > 0 {
		return fmt.Sprintf("%dh %02dm", h, minutes%60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func overlayText(t *Telemetry) string {
	z, left := "—", "—"
	if t != nil && t.AxisZ != nil {
		z = fmt.Sprintf("%.2f mm", *t.AxisZ)
	}
	if t != nil && t.Job != nil && t.Job.TimeRemaining != nil {
		left = formatTimeLeft(*t.Job.TimeRemaining)
	}
	return "REC  %{pts:gmtime:0:%H\\:%M\\:%S}\nZ    " + z + "\nLEFT " + left
}

var filterEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`, `,`, `\,`, `;`, `\;`, `[`, `\[`, `]`, `\]`)

func (r *recorder) videoArgs(textfile string) []string {
	codec := r.cfg.VideoCodec
	if textfile == "" {
		return []string{"-c:v", codec}
	}

	font := r.cfg.Font
	if font == "" {
		font = defaultFont
	}
	face := "font=monospace"
	if _, err := os.Stat(font); err == nil {
		face = "fontfile=" + filterEscaper.Replace(font)
	}
	drawtext := strings.Join([]string{
		"drawtext=" + face,
		"textfile=" + filterEscaper.Replace(textfile),
		"reload=25",
		"x=16:y=16:fontsize=26:line_spacing=6:fontcolor=white",
		"box=1:boxcolor=black@0.55:boxborderw=10",
	}, ":")
	if codec == "copy" {
		codec = "libx264"
	}
	return []string{"-vf", drawtext, "-c:v", codec, "-preset", "veryfast", "-crf", "26",
		"-g", "50", "-pix_fmt", "yuv420p", "-threads", "2"}
}

func writeOverlay(path, text string) {
	if os.WriteFile(path+".tmp", []byte(text), 0o644) == nil {
		_ = os.Rename(path+".tmp", path)
	}
}

func (r *recorder) pollOverlay(path string) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			t := r.link.status(ctx)
			if ctx.Err() != nil {
				return
			}
			if t.Online {
				writeOverlay(path, overlayText(&t))
			} else {
				writeOverlay(path, overlayText(nil))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(overlayPoll):
			}
		}
	}()
	return func() {
		cancel()
		_ = os.Remove(path)
	}
}

func makePoster(path string) {
	out := strings.TrimSuffix(path, ".mp4") + ".jpg"
	for _, seek := range [][]string{{"-ss", "1"}, {}} {
		args := append([]string{"-loglevel", "error"}, seek...)
		_ = exec.Command("ffmpeg", append(args, "-i", path, "-frames:v", "1", "-q:v", "5", "-y", out)...).Run()
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			return
		}
	}
	_ = os.Remove(out)
}
