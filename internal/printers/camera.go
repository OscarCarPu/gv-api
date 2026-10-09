package printers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	cameraFPS     = "5"
	cameraIdle    = 30 * time.Second
	cameraMaxBuf  = 4_000_000
	cameraTimeout = 5 * time.Second
)

var (
	jpegStart = []byte{0xff, 0xd8}
	jpegEnd   = []byte{0xff, 0xd9}
)

type camera struct {
	url     string
	mu      sync.Mutex
	latest  []byte
	seq     uint64
	changed chan struct{}
	last    time.Time
	running bool
}

func newCamera(url string) *camera {
	return &camera{url: url, changed: make(chan struct{})}
}

// next returns the first frame newer than seq, starting the stream if nobody is watching.
func (c *camera) next(ctx context.Context, seq uint64) ([]byte, uint64, error) {
	c.mu.Lock()
	c.last = time.Now()
	if !c.running {
		c.running = true
		go c.run()
	}
	c.mu.Unlock()

	timeout := time.After(cameraTimeout)
	for {
		c.mu.Lock()
		frame, cur, changed := c.latest, c.seq, c.changed
		c.mu.Unlock()
		if frame != nil && cur != seq {
			return frame, cur, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timeout:
			return nil, 0, errors.New("no frame available")
		}
	}
}

func (c *camera) idle() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.last) >= cameraIdle
}

func (c *camera) run() {
	for {
		c.stream()
		c.mu.Lock()
		c.latest = nil
		if time.Since(c.last) >= cameraIdle {
			c.running = false
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		time.Sleep(time.Second)
	}
}

func (c *camera) stream() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-rtsp_transport", "tcp",
		"-i", c.url, "-an", "-vf", "fps="+cameraFPS, "-f", "mjpeg", "-q:v", "6", "pipe:1")
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}

	go func() {
		for !c.idle() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		cancel()
	}()

	var pending []byte
	buf := make([]byte, 64<<10)
	for {
		n, err := out.Read(buf)
		pending = append(pending, buf[:n]...)
		for {
			start := bytes.Index(pending, jpegStart)
			if start < 0 {
				break
			}
			end := bytes.Index(pending[start+2:], jpegEnd)
			if end < 0 {
				break
			}
			end += start + 4
			frame := append([]byte(nil), pending[start:end]...)
			c.mu.Lock()
			c.latest = frame
			c.seq++
			close(c.changed)
			c.changed = make(chan struct{})
			c.mu.Unlock()
			pending = pending[end:]
		}
		if len(pending) > cameraMaxBuf {
			pending = nil
		}
		if err != nil {
			if err != io.EOF {
				cancel()
			}
			break
		}
	}
	_ = cmd.Wait()
}
