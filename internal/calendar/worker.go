package calendar

import (
	"context"
	"log/slog"
	"time"
)

/*
Worker is the calendar domain's background loop, and the first one in this API.

It does three things:

  - drains the change queue that webhooks fill, debounced per calendar, which is what makes
    updates arrive in seconds;
  - polls every SyncInterval, which is the safety net — Google states plainly that push
    notifications are not 100% reliable, and a channel can also die quietly;
  - keeps the push channels alive, since they expire and cannot be renewed in place.

The debounce is not a nicety: one change in Google produces several notifications (and one per
attendee who responds), and without it each would spend a sync token round-trip of its own.
*/
type Worker struct {
	svc      *Service
	interval time.Duration
	debounce time.Duration
}

func NewWorker(svc *Service) *Worker {
	return &Worker{
		svc:      svc,
		interval: svc.cfg.SyncInterval,
		debounce: svc.cfg.Debounce,
	}
}

// Run blocks until the context is cancelled. It returns rather than starting anything when
// Google is not configured, so the API runs fine with no credentials.
func (w *Worker) Run(ctx context.Context) {
	if !w.svc.Configured() {
		slog.Info("calendar: google is not configured, background sync disabled")
		return
	}
	slog.Info("calendar: background sync starting",
		"poll", w.interval, "debounce", w.debounce, "webhooks", w.svc.cfg.WebhookEnabled)

	// A first pass on boot: channels may have expired while the process was down, and the
	// local copy is however stale the downtime was.
	w.reconcile(ctx)

	poll := time.NewTicker(w.interval)
	defer poll.Stop()
	// The flush tick is what turns "a notification arrived" into "sync it shortly", so it
	// runs at a fraction of the debounce window.
	flush := time.NewTicker(max(w.debounce/2, 250*time.Millisecond))
	defer flush.Stop()

	pending := map[int32]time.Time{}

	for {
		select {
		case <-ctx.Done():
			slog.Info("calendar: background sync stopped")
			return

		case id := <-w.svc.Changes():
			pending[id] = time.Now().Add(w.debounce)

		case <-flush.C:
			now := time.Now()
			for id, due := range pending {
				if now.Before(due) {
					continue
				}
				delete(pending, id)
				go w.syncOne(ctx, id)
			}

		case <-poll.C:
			w.reconcile(ctx)
		}
	}
}

func (w *Worker) syncOne(ctx context.Context, calendarID int32) {
	res, err := w.svc.SyncCalendar(ctx, calendarID, "webhook")
	if err != nil {
		slog.ErrorContext(ctx, "calendar: push-triggered sync failed", "calendar", calendarID, "error", err)
		return
	}
	if res.Upserted > 0 || res.Deleted > 0 {
		slog.InfoContext(ctx, "calendar: synced from push notification",
			"calendar", calendarID, "upserted", res.Upserted, "deleted", res.Deleted)
	}
}

func (w *Worker) reconcile(ctx context.Context) {
	if err := w.svc.EnsureWatches(ctx); err != nil {
		slog.ErrorContext(ctx, "calendar: ensuring push channels", "error", err)
	}
	res, err := w.svc.SyncAll(ctx, "poll")
	if err != nil {
		slog.ErrorContext(ctx, "calendar: poll sync failed", "error", err)
		return
	}
	if len(res.Errors) > 0 {
		slog.WarnContext(ctx, "calendar: poll sync finished with errors",
			"calendars", res.Calendars, "errors", res.Errors)
	}
	if res.Upserted > 0 || res.Deleted > 0 {
		slog.InfoContext(ctx, "calendar: poll sync",
			"calendars", res.Calendars, "upserted", res.Upserted, "deleted", res.Deleted)
	}
}
