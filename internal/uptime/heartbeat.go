package uptime

import (
	"context"
	"log/slog"
	"time"
)

type Beater interface {
	Beat(ctx context.Context, now time.Time, gap time.Duration) error
}

type Heartbeat struct {
	beater Beater
	every  time.Duration
	gap    time.Duration
	gate   func(context.Context) bool
}

func NewHeartbeat(b Beater, every, gap time.Duration) *Heartbeat {
	return &Heartbeat{beater: b, every: every, gap: gap}
}

func (h *Heartbeat) Gated(gate func(context.Context) bool) *Heartbeat {
	h.gate = gate
	return h
}

func (h *Heartbeat) Run(ctx context.Context) {
	ticker := time.NewTicker(h.every)
	defer ticker.Stop()
	for {
		h.beat(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Heartbeat) beat(ctx context.Context) {
	if h.gate != nil && !h.gate(ctx) {
		return
	}
	if err := h.beater.Beat(ctx, time.Now(), h.gap); err != nil {
		slog.ErrorContext(ctx, "uptime: heartbeat failed", "error", err)
	}
}
