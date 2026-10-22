package failover

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"gv-api/internal/core"
	"gv-api/internal/report"

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

// ReportRoutes mounts the report screen's API. Mount it behind the normal login.
func (c *Controller) ReportRoutes(r chi.Router) {
	r.Get("/report", c.handleReport)
	r.Delete("/report", c.handleDismiss)
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

type reportResponse struct {
	statusResponse
	Report *report.Report `json:"report"`
}

func (c *Controller) handleReport(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	out := reportResponse{statusResponse{Mode: c.mode, Merge: c.merge, Error: c.err}, c.report}
	c.mu.Unlock()
	core.JSON(w, http.StatusOK, out)
}

func (c *Controller) handleDismiss(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.report = nil
	_ = os.Remove(c.reportFile())
	c.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) reportFile() string { return filepath.Join(c.opt.Dir, "report.json") }

func (c *Controller) loadReport() {
	raw, err := os.ReadFile(c.reportFile())
	if err != nil {
		return
	}
	var rep report.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		slog.Warn("ignoring unreadable report file", "error", err)
		return
	}
	c.report = &rep
}

// saveReport keeps the report across a restart; the caller holds the lock.
func (c *Controller) saveReport() {
	raw, err := json.Marshal(c.report)
	if err == nil {
		err = os.MkdirAll(c.opt.Dir, 0o700)
	}
	if err == nil {
		err = os.WriteFile(c.reportFile(), raw, 0o600)
	}
	if err != nil {
		slog.Warn("cannot save the report", "error", err)
	}
}
