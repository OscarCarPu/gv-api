package printers

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const autoRecordPoll = 5 * time.Second

func printInProgress(state string) bool {
	switch strings.ToUpper(state) {
	case "PRINTING", "PAUSED":
		return true
	}
	return false
}

type autoRecorder struct {
	printing bool
	known    bool
	started  bool
}

// observe takes one reading and says what to do about the recording. The first reading only sets
// the baseline, so a restart in the middle of a print does not begin a recording halfway through.
// An offline printer changes nothing: a dropped poll must not cut a recording short.
func (a *autoRecorder) observe(t Telemetry) (start, stop bool) {
	if !t.Online {
		return false, false
	}
	now := printInProgress(t.State)
	was, known := a.printing, a.known
	a.printing, a.known = now, true
	if !known {
		return false, false
	}
	return now && !was, !now && was && a.started
}

// Run records every print on its own: it starts when a print starts and stops when it ends,
// but only stops a recording it started itself.
func (s *Service) Run(ctx context.Context) {
	for _, u := range s.units {
		if u.p.Host != "" && u.p.RTSP != "" {
			go u.watch(ctx)
		}
	}
}

func (u *unit) watch(ctx context.Context) {
	var a autoRecorder
	tick := time.NewTicker(autoRecordPoll)
	defer tick.Stop()
	for {
		t := u.link.status(ctx)
		if ctx.Err() != nil {
			return
		}
		start, stop := a.observe(t)
		if start {
			if _, err := u.rec.start(); err != nil {
				slog.Warn("printers: auto-record did not start", "printer", u.p.ID, "error", err)
			} else {
				a.started = true
			}
		}
		if stop {
			a.started = false
			if err := u.rec.stop(); err != nil {
				slog.Info("printers: auto-record already stopped", "printer", u.p.ID, "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
