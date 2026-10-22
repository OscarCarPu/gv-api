package failover

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"gv-api/internal/core"
)

type Outages interface {
	RecordFailover(ctx context.Context, lastAlive *time.Time, ec2UpAt, homeAt time.Time) error
}

func (c *Controller) SetOutages(o Outages) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outages = o
}

// Beating lets the uptime heartbeat write only while home serves and the AWS state is NORMAL.
// The Lambda keeps the state at HOME_READY until the failover record is saved.
func (c *Controller) Beating(ctx context.Context) bool {
	if c.Mode() != ModeNormal {
		return false
	}
	state, err := c.state(ctx)
	return err != nil || state == stateNormal
}

func (c *Controller) handleOutage(w http.ResponseWriter, r *http.Request) {
	if !c.authorized(r) {
		core.Error(w, http.StatusUnauthorized, "invalid failover token")
		return
	}
	var body struct {
		EC2UpAt time.Time `json:"ec2_up_at"`
		HomeAt  time.Time `json:"home_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.EC2UpAt.IsZero() || !body.EC2UpAt.Before(body.HomeAt) || body.HomeAt.After(time.Now().Add(time.Minute)) {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	state, err := c.state(r.Context())
	if err != nil {
		core.InternalError(w, r, err, "cannot read the failover state")
		return
	}
	if state != stateHomeReady {
		core.Error(w, http.StatusConflict, "the failover state is "+state)
		return
	}

	c.mu.Lock()
	outages, lastAlive := c.outages, c.lastAlive
	c.mu.Unlock()
	if outages == nil {
		core.Error(w, http.StatusServiceUnavailable, "uptime is not available")
		return
	}
	if err := outages.RecordFailover(r.Context(), lastAlive, body.EC2UpAt, body.HomeAt); err != nil {
		core.InternalError(w, r, err, "cannot record the failover")
		return
	}
	c.mu.Lock()
	c.lastAlive = nil
	c.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}
