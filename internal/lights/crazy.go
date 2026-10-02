package lights

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

/*
Crazy mode: a bulb sweeps brightness and colour temperature on its own until told to stop.

It runs here rather than in a client because it has to outlive the tab that started it, and
because every client would otherwise need its own copy of the timing.

Brightness makes a full round trip (max to min and back) every crazyBrightnessPeriod, and
temperature every crazyTempPeriod. The periods are coprime, so the two only line up again
after 20s and the pair never reads as one slow fade.

The sweep is computed from elapsed time, not from a tick count: a write that takes longer
than a step delays that frame instead of stretching the period.
*/
const (
	crazyBrightnessPeriod = 5 * time.Second
	crazyTempPeriod       = 4 * time.Second

	// About 3 BLE writes a second. At 300ms a bulb started failing within a minute and stopped
	// advertising until power-cycled.
	crazyStep = 600 * time.Millisecond

	// 0 reads as off on these bulbs.
	crazyMinBrightness = 1
	crazyMaxBrightness = 100

	// Consecutive failed frames before the sweep gives up.
	crazyMaxFailures = 5
)

type crazyRun struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu   sync.Mutex
	last State
}

func (r *crazyRun) set(state State) {
	r.mu.Lock()
	r.last = state
	r.mu.Unlock()
}

// snapshot answers reads during the sweep; a radio query would hold the bulb's lock and stutter it.
func (r *crazyRun) snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.last
	state.Crazy = true
	return state
}

type crazyModes struct {
	step time.Duration

	mu   sync.Mutex
	runs map[string]*crazyRun
}

func newCrazyModes() *crazyModes {
	return &crazyModes{step: crazyStep, runs: map[string]*crazyRun{}}
}

func (m *crazyModes) get(id string) (*crazyRun, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	return run, ok
}

// remove forgets run only if it is still the registered one, so it cannot evict its replacement.
func (m *crazyModes) remove(id string, run *crazyRun) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs[id] == run {
		delete(m.runs, id)
	}
}

func (s *Service) crazyState(id string) (State, bool) {
	run, ok := s.crazy.get(id)
	if !ok {
		return State{}, false
	}
	return run.snapshot(), true
}

// startCrazy switches the bulb on and starts the sweep; a no-op if already sweeping.
func (s *Service) startCrazy(ctx context.Context, light Light) State {
	if state, ok := s.crazyState(light.ID); ok {
		return state
	}

	// Power on synchronously so an unreachable bulb fails this request.
	on := true
	state := s.driver.Apply(ctx, light, Command{Type: CommandPower, On: &on})
	if !state.Online {
		return state
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	run := &crazyRun{cancel: cancel, done: make(chan struct{}), last: state}

	s.crazy.mu.Lock()
	if existing, raced := s.crazy.runs[light.ID]; raced {
		s.crazy.mu.Unlock()
		cancel()
		return existing.snapshot()
	}
	s.crazy.runs[light.ID] = run
	s.crazy.mu.Unlock()

	slog.Info("crazy mode on", "light", light.ID)
	go s.sweep(loopCtx, light, run)
	return run.snapshot()
}

// stopCrazy ends the sweep and waits for its last write, so nothing lands after the next
// command. It returns what the bulb was last told.
func (s *Service) stopCrazy(id string) (State, bool) {
	run, ok := s.crazy.get(id)
	if !ok {
		return State{}, false
	}
	run.cancel()
	<-run.done
	s.crazy.remove(id, run)

	run.mu.Lock()
	state := run.last
	run.mu.Unlock()
	state.Crazy = false
	slog.Info("crazy mode off", "light", id)
	return state, true
}

func (s *Service) sweep(ctx context.Context, light Light, run *crazyRun) {
	defer close(run.done)
	defer s.crazy.remove(light.ID, run)

	ticker := time.NewTicker(s.crazy.step)
	defer ticker.Stop()

	start := time.Now()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		elapsed := time.Since(start)
		state, ok := s.crazyFrame(ctx, light, elapsed)
		if ctx.Err() != nil {
			return
		}
		run.set(state)

		if ok {
			failures = 0
			continue
		}
		failures++
		if failures >= crazyMaxFailures {
			slog.Warn("crazy mode gave up, bulb stopped answering", "light", light.ID)
			return
		}
	}
}

func (s *Service) crazyFrame(ctx context.Context, light Light, elapsed time.Duration) (State, bool) {
	brightness := crazyBrightness(elapsed)
	state := s.driver.Apply(ctx, light, Command{Type: CommandBrightness, Value: &brightness})
	if !state.Online {
		return state, false
	}

	if light.SupportsColorTemp {
		kelvin := crazyKelvin(light, elapsed)
		state = s.driver.Apply(ctx, light, Command{Type: CommandColorTemp, Kelvin: &kelvin})
	}
	return state, state.Online
}

// crazyWave maps elapsed time to 1 -> 0 -> 1 over one period.
func crazyWave(elapsed, period time.Duration) float64 {
	phase := float64(elapsed%period) / float64(period)
	return math.Abs(2*phase - 1)
}

func crazyBrightness(elapsed time.Duration) int {
	span := float64(crazyMaxBrightness - crazyMinBrightness)
	return crazyMinBrightness + int(math.Round(crazyWave(elapsed, crazyBrightnessPeriod)*span))
}

func crazyKelvin(light Light, elapsed time.Duration) int {
	low, high := light.MinColorTemp, light.MaxColorTemp
	return int(math.Round(low + crazyWave(elapsed, crazyTempPeriod)*(high-low)))
}
