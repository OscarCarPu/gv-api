package backup

import (
	"context"
	"log/slog"
	"time"
)

type Worker struct {
	svc      *Service
	interval time.Duration
}

func NewWorker(svc *Service, interval time.Duration) *Worker {
	return &Worker{svc: svc, interval: interval}
}

func (w *Worker) Run(ctx context.Context) {
	if w.interval <= 0 {
		slog.Info("backup: schedule disabled")
		return
	}
	slog.Info("backup: schedule starting",
		"interval", w.interval, "keep_hourly", w.svc.cfg.KeepHourly, "keep_daily", w.svc.cfg.KeepDaily)

	for {
		timer := time.NewTimer(time.Until(time.Now().Truncate(w.interval).Add(w.interval)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:

		}

		b, err := w.svc.Run(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "backup: scheduled backup failed", "error", err)
			continue
		}
		slog.InfoContext(ctx, "backup: done", "file", b.Name, "size", b.Size)
	}
}
