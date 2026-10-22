package failover_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gv-api/internal/failover"
	"gv-api/internal/merge"
	"gv-api/internal/restore"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const token = "secret"

type params struct {
	mu   sync.Mutex
	vals map[string]string
	err  error
}

func (p *params) Get(_ context.Context, name string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return "", p.err
	}
	v, ok := p.vals[name]
	if !ok {
		return "", failover.ErrNoParam
	}
	return v, nil
}

func (p *params) set(name, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vals[name] = value
}

func state(s string) string { return `{"state":"` + s + `"}` }

type store struct {
	mu   sync.Mutex
	keys []string
}

func (s *store) Download(_ context.Context, key, dest string) error {
	s.mu.Lock()
	s.keys = append(s.keys, key)
	s.mu.Unlock()
	return os.WriteFile(dest, []byte(key), 0o600)
}

type restorer struct {
	mu      sync.Mutex
	runs    []restore.Request
	outcome func(call int) (*restore.Outcome, error)
}

func (r *restorer) Run(_ context.Context, req restore.Request) (*restore.Outcome, error) {
	r.mu.Lock()
	r.runs = append(r.runs, req)
	call := len(r.runs)
	r.mu.Unlock()
	return r.outcome(call)
}

func (r *restorer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

var swapped = func(int) (*restore.Outcome, error) { return &restore.Outcome{Swapped: true}, nil }

var clash = merge.Ref{Table: "parents", Key: "[2]"}

func conflicting(call int) (*restore.Outcome, error) {
	if call > 1 {
		return &restore.Outcome{Swapped: true}, nil
	}
	return &restore.Outcome{Result: &merge.Result{Conflicts: []merge.Conflict{{
		Ref: clash, Op: merge.Update, Reason: "same row edited on both sides",
		Base: map[string]any{"name": "two"}, Home: map[string]any{"name": "h"}, EC2: map[string]any{"name": "e"},
	}}}}, nil
}

type fixture struct {
	ctl    *failover.Controller
	params *params
	store  *store
	run    *restorer
	router http.Handler
	after  int
	dir    string
}

func newFixture(t *testing.T, stateJSON string, outcome func(int) (*restore.Outcome, error)) *fixture {
	t.Helper()
	f := &fixture{
		params: &params{vals: map[string]string{
			"/home-lab-failover/base": "gv-db/hourly/base.sql.gz",
			"/home-lab-failover/dump": "gv-db/hourly/dump.sql.gz",
		}},
		store: &store{},
		run:   &restorer{outcome: outcome},
		dir:   t.TempDir(),
	}
	if stateJSON != "" {
		f.params.set("/home-lab-failover/state", stateJSON)
	}
	f.ctl = f.controller(token)
	f.router = f.routes(f.ctl)
	return f
}

func (f *fixture) controller(tok string) *failover.Controller {
	return failover.New(failover.Options{
		Token: tok, Params: f.params, Store: f.store, Restorer: f.run, Dir: f.dir,
		After: func() { f.after++ }, Retry: 10 * time.Millisecond,
	})
}

func (f *fixture) routes(c *failover.Controller) http.Handler {
	r := chi.NewRouter()
	r.Use(c.Lock)
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/health", ok)
	r.Get("/habits", ok)
	r.Post("/login", ok)
	c.Routes(r)
	c.MergeRoutes(r)
	return r
}

func (f *fixture) do(method, path, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("X-Failover-Token", tok)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) merge(t *testing.T) failover.Merge {
	t.Helper()
	var s struct{ Merge failover.Merge }
	require.NoError(t, json.Unmarshal(f.do("GET", "/failover/status", token, "").Body.Bytes(), &s))
	return s.Merge
}

func (f *fixture) waitFor(t *testing.T, want failover.Merge) {
	t.Helper()
	require.Eventually(t, func() bool { return f.merge(t) == want }, 2*time.Second, 5*time.Millisecond)
}

func TestRoutes_NeedTheToken(t *testing.T) {
	f := newFixture(t, state("FAILED_OVER"), swapped)

	for _, tok := range []string{"", "wrong"} {
		assert.Equal(t, http.StatusUnauthorized, f.do("POST", "/failover", tok, `{"mode":"hold"}`).Code)
		assert.Equal(t, http.StatusUnauthorized, f.do("GET", "/failover/status", tok, "").Code)
	}
	assert.Equal(t, failover.ModeNormal, f.ctl.Mode())
}

func TestRoutes_AreNotMountedWithoutAToken(t *testing.T) {
	f := newFixture(t, state("FAILED_OVER"), swapped)
	off := f.controller("")

	assert.False(t, off.Enabled())
	f.router = f.routes(off)
	assert.Equal(t, http.StatusNotFound, f.do("POST", "/failover", "", `{"mode":"hold"}`).Code)
}

func TestHold_OnlyWhenTheStateIsNotNormal(t *testing.T) {
	f := newFixture(t, state("NORMAL"), swapped)

	assert.Equal(t, http.StatusConflict, f.do("POST", "/failover", token, `{"mode":"hold"}`).Code)
	assert.Equal(t, failover.ModeNormal, f.ctl.Mode())

	f.params.set("/home-lab-failover/state", state("FAILED_OVER"))
	assert.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"hold"}`).Code)
	assert.Equal(t, failover.ModeHold, f.ctl.Mode())
}

func TestLock_AnswersMergePendingExceptForTheRoutesTheFailoverNeeds(t *testing.T) {
	f := newFixture(t, state("FAILED_OVER"), swapped)
	assert.Equal(t, http.StatusOK, f.do("GET", "/habits", "", "").Code)

	require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"hold"}`).Code)

	rec := f.do("GET", "/habits", "", "")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"error":"Restoring data after the failover. Back in a moment.","code":"merge_pending"}`, rec.Body.String())
	for _, route := range [][2]string{{"GET", "/health"}, {"POST", "/login"}, {"GET", "/merge"}, {"GET", "/failover/status"}} {
		assert.NotEqual(t, http.StatusServiceUnavailable, f.do(route[0], route[1], token, "").Code, route[1])
	}
}

func TestMerge_NeedsTheRightStateAndADump(t *testing.T) {
	f := newFixture(t, state("FAILED_OVER"), swapped)

	assert.Equal(t, http.StatusConflict, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code, "state FAILED_OVER")

	f.params.set("/home-lab-failover/state", state("FAILING_BACK"))
	delete(f.params.vals, "/home-lab-failover/dump")
	assert.Equal(t, http.StatusConflict, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code, "no dump")

	assert.Equal(t, http.StatusBadRequest, f.do("POST", "/failover", token, `{"mode":"explode"}`).Code)
	assert.Zero(t, f.run.count())
}

func TestMerge_DownloadsBothDumpsRunsAndUnlocks(t *testing.T) {
	f := newFixture(t, state("FAILING_BACK"), swapped)

	rec := f.do("POST", "/failover", token, `{"mode":"merge"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	f.waitFor(t, failover.MergeDone)
	assert.Equal(t, failover.ModeNormal, f.ctl.Mode())
	assert.Equal(t, 1, f.after)
	assert.ElementsMatch(t, []string{"gv-db/hourly/base.sql.gz", "gv-db/hourly/dump.sql.gz"}, f.store.keys)
	req := f.run.runs[0]
	assert.Equal(t, "dump.sql.gz", req.ID)
	assert.True(t, req.Commit)
	assert.Equal(t, filepath.Join(f.dir, "downloads", "base.sql.gz"), req.Base)
	assert.Equal(t, filepath.Join(f.dir, "downloads", "dump.sql.gz"), req.EC2)
}

func TestMerge_AConflictWaitsForAnswersThenFinishes(t *testing.T) {
	f := newFixture(t, state("FAILING_BACK"), conflicting)
	require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code)
	f.waitFor(t, failover.MergeNeedsAnswers)
	assert.Equal(t, failover.ModeMerge, f.ctl.Mode(), "still locked")

	var got struct {
		Conflicts []map[string]any
	}
	require.NoError(t, json.Unmarshal(f.do("GET", "/merge", "", "").Body.Bytes(), &got))
	require.Len(t, got.Conflicts, 1)
	assert.Equal(t, "parents", got.Conflicts[0]["table"])
	assert.Equal(t, "[2]", got.Conflicts[0]["key"])

	assert.Equal(t, http.StatusConflict, f.do("POST", "/merge/apply", "", "").Code, "unanswered")
	assert.Equal(t, http.StatusNotFound, f.do("POST", "/merge/decisions", "", `{"table":"x","key":"[1]","choice":"home"}`).Code)
	assert.Equal(t, http.StatusBadRequest, f.do("POST", "/merge/decisions", "", `{"table":"parents","key":"[2]","choice":"both"}`).Code)
	require.Equal(t, http.StatusNoContent, f.do("POST", "/merge/decisions", "", `{"table":"parents","key":"[2]","choice":"home"}`).Code)

	assert.Equal(t, http.StatusAccepted, f.do("POST", "/merge/apply", "", "").Code)
	f.waitFor(t, failover.MergeDone)
	assert.Equal(t, failover.ModeNormal, f.ctl.Mode())
	assert.Equal(t, merge.Home, f.run.runs[1].Decisions[clash], "the second run carries the answer")
}

func TestMerge_AFailureIsReportedAndCanBeRetried(t *testing.T) {
	f := newFixture(t, state("FAILING_BACK"), func(call int) (*restore.Outcome, error) {
		if call == 1 {
			return nil, errors.New("disk full")
		}
		return &restore.Outcome{Swapped: true}, nil
	})
	require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code)
	f.waitFor(t, failover.MergeFailed)
	assert.Contains(t, f.do("GET", "/failover/status", token, "").Body.String(), "disk full")
	assert.Equal(t, failover.ModeMerge, f.ctl.Mode())

	assert.Equal(t, http.StatusAccepted, f.do("POST", "/merge/apply", "", "").Code)
	f.waitFor(t, failover.MergeDone)
}

func TestMerge_DoesNotRunTwiceAtOnce(t *testing.T) {
	release := make(chan struct{})
	f := newFixture(t, state("FAILING_BACK"), func(int) (*restore.Outcome, error) {
		<-release
		return &restore.Outcome{Swapped: true}, nil
	})
	for range 3 {
		require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code)
	}
	close(release)
	f.waitFor(t, failover.MergeDone)

	assert.Equal(t, 1, f.run.count())
}

func TestStart(t *testing.T) {
	tests := []struct {
		name  string
		state string
		mode  failover.Mode
		runs  int
	}{
		{"normal", state("NORMAL"), failover.ModeNormal, 0},
		{"no state yet is normal", "", failover.ModeNormal, 0},
		{"failed over holds the app", state("FAILED_OVER"), failover.ModeHold, 0},
		{"failing back holds the app", state("FAILING_BACK"), failover.ModeHold, 0},
		{"pending merge resumes the merge", state("PENDING_MERGE"), failover.ModeMerge, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.state, func(int) (*restore.Outcome, error) {
				time.Sleep(50 * time.Millisecond)
				return &restore.Outcome{Swapped: true}, nil
			})

			f.ctl.Start(context.Background())

			assert.Equal(t, tt.mode, f.ctl.Mode())
			if tt.runs > 0 {
				f.waitFor(t, failover.MergeDone)
			}
			assert.Equal(t, tt.runs, f.run.count())
		})
	}
}

func TestStart_HoldsWhileTheStateIsUnreadableThenLetsGo(t *testing.T) {
	f := newFixture(t, state("NORMAL"), swapped)
	f.params.err = errors.New("no network")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.ctl.Start(ctx)

	assert.Equal(t, failover.ModeHold, f.ctl.Mode())
	f.params.mu.Lock()
	f.params.err = nil
	f.params.mu.Unlock()
	require.Eventually(t, func() bool { return f.ctl.Mode() == failover.ModeNormal }, 2*time.Second, 5*time.Millisecond)
}

func TestAllowed(t *testing.T) {
	f := newFixture(t, state("NORMAL"), swapped)
	ctx := context.Background()
	assert.True(t, f.ctl.Allowed(ctx))

	f.params.err = errors.New("no network")
	assert.True(t, f.ctl.Allowed(ctx), "an unreadable state does not stop the worker")
	f.params.err = nil

	f.params.set("/home-lab-failover/state", state("FAILED_OVER"))
	assert.False(t, f.ctl.Allowed(ctx))
	assert.Equal(t, failover.ModeHold, f.ctl.Mode(), "a failover the push never reached holds the app")
	assert.False(t, f.ctl.Allowed(ctx))
}

func TestDecisionsSurviveARestart(t *testing.T) {
	f := newFixture(t, state("FAILING_BACK"), conflicting)
	require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code)
	f.waitFor(t, failover.MergeNeedsAnswers)
	require.Equal(t, http.StatusNoContent, f.do("POST", "/merge/decisions", "", `{"table":"parents","key":"[2]","choice":"ec2"}`).Code)

	restarted := f.controller(token)
	f.router = f.routes(restarted)
	restarted.Start(context.Background())
	f.params.set("/home-lab-failover/state", state("PENDING_MERGE"))
	restarted.Start(context.Background())

	require.Eventually(t, func() bool { return f.run.count() >= 2 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, merge.EC2, f.run.runs[1].Decisions[clash])
}
