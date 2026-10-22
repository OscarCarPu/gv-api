package failover

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"gv-api/internal/core"
	"gv-api/internal/merge"

	"github.com/go-chi/chi/v5"
)

// Routes mounts what the AWS side calls. They answer to the failover token, not to a user login.
func (c *Controller) Routes(r chi.Router) {
	if !c.Enabled() {
		return
	}
	r.Post("/failover", c.handleFailover)
	r.Get("/failover/status", c.handleStatus)
}

// MergeRoutes mounts the conflict screen's API. Mount it behind the normal login.
func (c *Controller) MergeRoutes(r chi.Router) {
	r.Get("/merge", c.handleMerge)
	r.Post("/merge/decisions", c.handleDecision)
	r.Post("/merge/apply", c.handleApply)
}

func (c *Controller) handleFailover(w http.ResponseWriter, r *http.Request) {
	if !c.authorized(r) {
		core.Error(w, http.StatusUnauthorized, "invalid failover token")
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	state, err := c.state(r.Context())
	if err != nil {
		core.InternalError(w, r, err, "cannot read the failover state")
		return
	}

	switch body.Mode {
	case "hold":
		if state == stateNormal {
			core.Error(w, http.StatusConflict, "the failover state is NORMAL")
			return
		}
		c.setMode(ModeHold)
	case "merge":
		if state != stateFailingBack && state != statePending {
			core.Error(w, http.StatusConflict, "the failover state is "+state)
			return
		}
		if _, err := c.opt.Params.Get(r.Context(), dumpParam); err != nil {
			core.Error(w, http.StatusConflict, "no failback dump yet")
			return
		}
		c.startMerge()
	default:
		core.Error(w, http.StatusBadRequest, "mode must be hold or merge")
		return
	}
	core.JSON(w, http.StatusOK, c.status())
}

func (c *Controller) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !c.authorized(r) {
		core.Error(w, http.StatusUnauthorized, "invalid failover token")
		return
	}
	core.JSON(w, http.StatusOK, c.status())
}

type conflictResponse struct {
	Table  string         `json:"table"`
	Key    string         `json:"key"`
	Op     merge.Op       `json:"op"`
	Reason string         `json:"reason"`
	Base   map[string]any `json:"base"`
	Home   map[string]any `json:"home"`
	EC2    map[string]any `json:"ec2"`
	Choice merge.Choice   `json:"choice,omitempty"`
}

type mergeResponse struct {
	statusResponse
	Conflicts []conflictResponse `json:"conflicts"`
}

func (c *Controller) handleMerge(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	out := mergeResponse{
		statusResponse: statusResponse{Mode: c.mode, Merge: c.merge, Error: c.err},
		Conflicts:      []conflictResponse{},
	}
	for _, k := range c.conflicts {
		out.Conflicts = append(out.Conflicts, conflictResponse{
			Table: k.Table, Key: k.Key, Op: k.Op, Reason: k.Reason,
			Base: k.Base, Home: k.Home, EC2: k.EC2, Choice: c.decisions[k.Ref],
		})
	}
	c.mu.Unlock()
	core.JSON(w, http.StatusOK, out)
}

func (c *Controller) handleDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Table  string       `json:"table"`
		Key    string       `json:"key"`
		Choice merge.Choice `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Table == "" || body.Key == "" {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Choice != merge.Home && body.Choice != merge.EC2 {
		core.Error(w, http.StatusBadRequest, "choice must be home or ec2")
		return
	}

	c.mu.Lock()
	known := false
	for _, k := range c.conflicts {
		known = known || (k.Table == body.Table && k.Key == body.Key)
	}
	if !known {
		c.mu.Unlock()
		core.Error(w, http.StatusNotFound, "no such conflict")
		return
	}
	c.decisions[merge.Ref{Table: body.Table, Key: body.Key}] = body.Choice
	c.saveDecisions()
	c.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) handleApply(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	if c.merge != MergeNeedsAnswers && c.merge != MergeFailed {
		status := c.merge
		c.mu.Unlock()
		core.Error(w, http.StatusConflict, "nothing to apply while the merge is "+string(status))
		return
	}
	for _, k := range c.conflicts {
		if c.decisions[k.Ref] == "" {
			c.mu.Unlock()
			core.Error(w, http.StatusConflict, "answer every conflict first")
			return
		}
	}
	c.mu.Unlock()

	c.startMerge()
	w.WriteHeader(http.StatusAccepted)
}

type storedDecision struct {
	Table  string       `json:"table"`
	Key    string       `json:"key"`
	Choice merge.Choice `json:"choice"`
}

func (c *Controller) decisionsFile() string { return filepath.Join(c.opt.Dir, "decisions.json") }

func (c *Controller) loadDecisions() {
	raw, err := os.ReadFile(c.decisionsFile())
	if err != nil {
		return
	}
	var stored []storedDecision
	if err := json.Unmarshal(raw, &stored); err != nil {
		slog.Warn("ignoring unreadable decisions file", "error", err)
		return
	}
	for _, d := range stored {
		c.decisions[merge.Ref{Table: d.Table, Key: d.Key}] = d.Choice
	}
}

// saveDecisions keeps the answers across a restart; the caller holds the lock.
func (c *Controller) saveDecisions() {
	var stored []storedDecision
	for ref, choice := range c.decisions {
		stored = append(stored, storedDecision{Table: ref.Table, Key: ref.Key, Choice: choice})
	}
	raw, err := json.Marshal(stored)
	if err == nil {
		err = os.MkdirAll(c.opt.Dir, 0o700)
	}
	if err == nil {
		err = os.WriteFile(c.decisionsFile(), raw, 0o600)
	}
	if err != nil {
		slog.Warn("cannot save the decisions", "error", err)
	}
}
