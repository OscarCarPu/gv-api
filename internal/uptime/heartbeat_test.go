package uptime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gv-api/internal/uptime"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBeater struct {
	beats atomic.Int32
	err   error
}

func (f *fakeBeater) Beat(context.Context, time.Time, time.Duration) error {
	f.beats.Add(1)
	return f.err
}

func run(t *testing.T, h *uptime.Heartbeat) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the heartbeat did not stop")
	}
}

func TestHeartbeat_BeatsAtOnceAndThenOnEveryTick(t *testing.T) {
	b := &fakeBeater{}

	run(t, uptime.NewHeartbeat(b, 20*time.Millisecond, uptime.Gap))

	assert.GreaterOrEqual(t, int(b.beats.Load()), 3)
}

func TestHeartbeat_ABeatRightAtStartEvenWithALongInterval(t *testing.T) {
	b := &fakeBeater{}

	run(t, uptime.NewHeartbeat(b, time.Hour, uptime.Gap))

	assert.Equal(t, int32(1), b.beats.Load())
}

func TestHeartbeat_AClosedGateWritesNothing(t *testing.T) {
	b := &fakeBeater{}
	h := uptime.NewHeartbeat(b, 20*time.Millisecond, uptime.Gap).Gated(func(context.Context) bool { return false })

	run(t, h)

	assert.Zero(t, b.beats.Load())
}

func TestHeartbeat_AFailedBeatDoesNotStopIt(t *testing.T) {
	b := &fakeBeater{err: errors.New("db down")}

	run(t, uptime.NewHeartbeat(b, 20*time.Millisecond, uptime.Gap))

	require.GreaterOrEqual(t, int(b.beats.Load()), 3)
}
