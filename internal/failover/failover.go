// Package failover is home's side of an AWS failover: it holds the app while the backup server
// owns the data, and merges what that server wrote back into home's database.
//
// The AWS state machine is the authority. Requests only say what to do; every one is checked
// against the state stored in SSM, so a leaked token cannot start a merge or choose a dump.
package failover

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gv-api/internal/core"
	"gv-api/internal/merge"
	"gv-api/internal/restore"
)

const (
	CodeMergePending = "merge_pending"
	messagePending   = "Restoring data after the failover. Back in a moment."

	stateParam = "/home-lab-failover/state"
	baseParam  = "/home-lab-failover/base"
	dumpParam  = "/home-lab-failover/dump"

	stateNormal      = "NORMAL"
	stateFailingBack = "FAILING_BACK"
	statePending     = "PENDING_MERGE"
)

var ErrNoParam = errors.New("parameter not found")

type Params interface {
	Get(ctx context.Context, name string) (string, error)
}

type Store interface {
	Download(ctx context.Context, key, dest string) error
}

type Restorer interface {
	Run(ctx context.Context, req restore.Request) (*restore.Outcome, error)
}

type Mode string

const (
	ModeNormal Mode = "normal"
	ModeHold   Mode = "hold"
	ModeMerge  Mode = "merge"
)

type Merge string

const (
	MergeIdle         Merge = ""
	MergeRunning      Merge = "running"
	MergeNeedsAnswers Merge = "needs_answers"
	MergeDone         Merge = "done"
	MergeFailed       Merge = "failed"
)

type Options struct {
	Token    string
	Params   Params
	Store    Store
	Restorer Restorer
	// Dir keeps the downloaded dumps and the answers given so far.
	Dir string
	// After runs once a merge swapped the database, before the app unlocks.
	After func()
	// Retry is how often an unreadable state is tried again while the app is held.
	Retry time.Duration
}

type Controller struct {
	opt Options
	ctx context.Context

	mu        sync.Mutex
	mode      Mode
	merge     Merge
	err       string
	conflicts []merge.Conflict
	decisions map[merge.Ref]merge.Choice
	running   bool
}

func New(opt Options) *Controller {
	if opt.Retry == 0 {
		opt.Retry = 30 * time.Second
	}
	c := &Controller{opt: opt, ctx: context.Background(), mode: ModeNormal, decisions: map[merge.Ref]merge.Choice{}}
	c.loadDecisions()
	return c
}

// Enabled is false without a token: the routes are then not mounted at all.
func (c *Controller) Enabled() bool { return c.opt.Token != "" }

// Start reads the state once. Anything but NORMAL holds the app; PENDING_MERGE also resumes the
// merge. When the state cannot be read the app is held and the read is retried until it works.
func (c *Controller) Start(ctx context.Context) {
	c.ctx = ctx
	state, err := c.state(ctx)
	if err != nil {
		slog.Error("failover state unreadable, holding the app", "error", err)
		c.setMode(ModeHold)
		go c.retry(ctx)
		return
	}
	c.apply(state)
}

func (c *Controller) apply(state string) {
	switch state {
	case stateNormal:
		c.setMode(ModeNormal)
	case statePending:
		c.startMerge()
	default:
		c.setMode(ModeHold)
	}
}

func (c *Controller) retry(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.opt.Retry):
		}
		state, err := c.state(ctx)
		if err != nil {
			continue
		}
		c.apply(state)
		return
	}
}

// Allowed tells a worker whether it may run. It rereads the state, which also catches a failover
// the push never reached. A state it cannot read does not block the worker: without SSM there is
// no S3 either, so nothing stale can be uploaded.
func (c *Controller) Allowed(ctx context.Context) bool {
	if c.Mode() != ModeNormal {
		return false
	}
	state, err := c.state(ctx)
	if err != nil {
		slog.Warn("failover state unreadable, letting the worker run", "error", err)
		return true
	}
	if state != stateNormal {
		c.setMode(ModeHold)
		return false
	}
	return true
}

func (c *Controller) Mode() Mode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

func (c *Controller) setMode(m Mode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m == ModeHold && (c.merge == MergeDone || c.merge == MergeFailed) {
		c.merge, c.err = MergeIdle, ""
	}
	c.mode = m
}

func (c *Controller) state(ctx context.Context) (string, error) {
	raw, err := c.opt.Params.Get(ctx, stateParam)
	if errors.Is(err, ErrNoParam) {
		return stateNormal, nil
	}
	if err != nil {
		return "", err
	}
	var s struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", err
	}
	return s.State, nil
}

// Lock answers 503 on everything but the routes the failover itself needs, while the app is held.
func (c *Controller) Lock(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.Mode() == ModeNormal || exempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		core.ErrorCode(w, http.StatusServiceUnavailable, CodeMergePending, messagePending)
	})
}

func exempt(p string) bool {
	for _, prefix := range []string{"/health", "/login", "/failover", "/merge"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

func (c *Controller) startMerge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running || c.merge == MergeDone {
		return
	}
	c.running, c.mode, c.merge, c.err = true, ModeMerge, MergeRunning, ""
	go c.runMerge()
}

func (c *Controller) runMerge() {
	out, err := c.restore()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	switch {
	case err != nil:
		slog.Error("merge failed", "error", err)
		c.merge, c.err = MergeFailed, err.Error()
	case out.Swapped:
		if c.opt.After != nil {
			c.opt.After()
		}
		c.merge, c.mode, c.conflicts = MergeDone, ModeNormal, nil
	case out.Result != nil && len(out.Result.Conflicts) > 0:
		c.merge, c.conflicts = MergeNeedsAnswers, out.Result.Conflicts
	default:
		c.merge, c.err = MergeFailed, "the merge ended without a swap"
	}
}

func (c *Controller) restore() (*restore.Outcome, error) {
	baseKey, err := c.opt.Params.Get(c.ctx, baseParam)
	if err != nil {
		return nil, err
	}
	dumpKey, err := c.opt.Params.Get(c.ctx, dumpParam)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(c.opt.Dir, "downloads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base, ec2 := filepath.Join(dir, path.Base(baseKey)), filepath.Join(dir, path.Base(dumpKey))
	for key, dest := range map[string]string{baseKey: base, dumpKey: ec2} {
		if err := c.opt.Store.Download(c.ctx, key, dest); err != nil {
			return nil, err
		}
	}

	c.mu.Lock()
	decisions := make(map[merge.Ref]merge.Choice, len(c.decisions))
	for ref, choice := range c.decisions {
		decisions[ref] = choice
	}
	c.mu.Unlock()

	return c.opt.Restorer.Run(c.ctx, restore.Request{
		ID: path.Base(dumpKey), Base: base, EC2: ec2, Decisions: decisions, Commit: true,
	})
}

func (c *Controller) authorized(r *http.Request) bool {
	got := []byte(r.Header.Get("X-Failover-Token"))
	return c.opt.Token != "" && subtle.ConstantTimeCompare(got, []byte(c.opt.Token)) == 1
}

type statusResponse struct {
	Mode  Mode   `json:"mode"`
	Merge Merge  `json:"merge"`
	Error string `json:"error,omitempty"`
}

func (c *Controller) status() statusResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return statusResponse{Mode: c.mode, Merge: c.merge, Error: c.err}
}
