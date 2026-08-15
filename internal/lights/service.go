package lights

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"
)

// ErrNotFound is returned for an unknown bulb id; the handler maps it to 404.
var ErrNotFound = errors.New("light not found")

// ServiceInterface is the seam the handler depends on, so it can be mocked in tests.
type ServiceInterface interface {
	List() []PublicLight
	States(ctx context.Context, force bool) []State
	State(ctx context.Context, id string, force bool) (State, error)
	Send(ctx context.Context, id string, cmd Command) (State, error)
}

// Service reads and writes bulbs through a Driver, with a short read cache.
//
// The cache exists because BLE is slow and serialises badly: a read is hundreds of
// milliseconds at best, clients poll every few seconds, and two overlapping reads of the same
// bulb tend to fail both. Reads inside the TTL are served from memory, and concurrent reads of
// one bulb share a single in-flight call. Writes bypass the cache and replace it with their
// result.
type Service struct {
	registry *Registry
	driver   Driver
	ttl      time.Duration

	// How many times a write is re-applied when the bulb drifts off the requested value,
	// and how long to let it transition before checking. Zero attempts disables settling.
	settleAttempts int
	settleDelay    time.Duration

	mu     sync.Mutex
	cache  map[string]cacheEntry
	flight map[string]*inflight
}

type cacheEntry struct {
	state State
	at    time.Time
}

// inflight lets late callers wait on an in-progress read instead of starting their own.
type inflight struct {
	done  chan struct{}
	state State
}

func NewService(registry *Registry, driver Driver, ttl time.Duration, settleAttempts int, settleDelay time.Duration) *Service {
	if ttl < 0 {
		ttl = 0
	}
	if settleAttempts < 0 {
		settleAttempts = 0
	}
	if settleDelay <= 0 {
		settleDelay = 400 * time.Millisecond
	}
	return &Service{
		registry:       registry,
		driver:         driver,
		ttl:            ttl,
		settleAttempts: settleAttempts,
		settleDelay:    settleDelay,
		cache:          map[string]cacheEntry{},
		flight:         map[string]*inflight{},
	}
}

func (s *Service) List() []PublicLight { return s.registry.Public() }

// States reads every bulb in parallel — each is an independent connection, and serialising
// them would multiply the worst case by the number of bulbs.
func (s *Service) States(ctx context.Context, force bool) []State {
	lights := s.registry.All()
	out := make([]State, len(lights))

	var wg sync.WaitGroup
	for i, light := range lights {
		wg.Add(1)
		go func(i int, light Config) {
			defer wg.Done()
			out[i] = s.read(ctx, light, force)
		}(i, light)
	}
	wg.Wait()
	return out
}

func (s *Service) State(ctx context.Context, id string, force bool) (State, error) {
	light, ok := s.registry.Get(id)
	if !ok {
		return State{}, ErrNotFound
	}
	return s.read(ctx, light, force), nil
}

func (s *Service) Send(ctx context.Context, id string, cmd Command) (State, error) {
	light, ok := s.registry.Get(id)
	if !ok {
		return State{}, ErrNotFound
	}
	if err := cmd.Validate(); err != nil {
		return State{}, err
	}

	state := s.driver.Apply(ctx, light, cmd)
	state = s.settle(ctx, light, cmd, state)
	s.store(state)
	return state, nil
}

/*
settle re-applies a command until the bulb actually holds the requested value.

These bulbs do not always land where they are told: a value arrives a little late, or the
lamp settles on a neighbouring step and stays there. Correcting that by hand is not the
client's job — and doing it in each client would mean three implementations of the same
retry. So the API closes the loop: write, wait for the lamp to transition, read back, and
write again if it drifted.

Only continuous values are settled. Power is a boolean the bulb either took or did not, and
colour is not supported by the hardware in use. A bulb that cannot be read back is left
alone — there is nothing to compare against, and re-writing blind would just be noise.
*/
func (s *Service) settle(ctx context.Context, light Config, cmd Command, state State) State {
	target, tolerance, ok := settleTarget(cmd)
	if !ok || !state.Online || s.settleAttempts <= 0 {
		return state
	}

	for range s.settleAttempts {
		select {
		case <-ctx.Done():
			return state
		case <-time.After(s.settleDelay):
		}

		fresh := s.driver.GetState(ctx, light)
		if !fresh.Online {
			// Lost the bulb mid-correction; report that rather than the value we hoped for.
			return fresh
		}

		actual, ok := settleActual(cmd, fresh)
		if !ok || math.Abs(actual-target) <= tolerance {
			return fresh
		}

		slog.Debug("light drifted, re-applying",
			"light", light.ID, "command", cmd.Type, "want", target, "got", actual)
		state = s.driver.Apply(ctx, light, cmd)
		if !state.Online {
			return state
		}
	}
	return state
}

// settleTarget returns the value a command asked for and how far off is close enough.
// ok is false for commands that cannot meaningfully be verified.
func settleTarget(cmd Command) (target, tolerance float64, ok bool) {
	switch cmd.Type {
	case CommandBrightness:
		// The bulbs' own scale is 0-254 against our 0-100, so a clean round-trip can still
		// differ by one after rounding in both directions. Anything more is real drift.
		return float64(*cmd.Value), 1, true
	case CommandColorTemp:
		// One mired step is ~13K and readback snaps to 10K, so ~2 steps of slack.
		return float64(*cmd.Kelvin), 30, true
	default:
		return 0, 0, false
	}
}

// settleActual pulls the comparable field out of a freshly read state.
func settleActual(cmd Command, state State) (float64, bool) {
	switch cmd.Type {
	case CommandBrightness:
		return state.Brightness, true
	case CommandColorTemp:
		return state.ColorTemp, true
	default:
		return 0, false
	}
}

func (s *Service) read(ctx context.Context, light Config, force bool) State {
	if !force {
		if state, ok := s.cached(light.ID); ok {
			return state
		}
	}

	// Join an in-progress read for this bulb rather than starting a competing one.
	s.mu.Lock()
	if existing, ok := s.flight[light.ID]; ok {
		s.mu.Unlock()
		<-existing.done
		return existing.state
	}
	call := &inflight{done: make(chan struct{})}
	s.flight[light.ID] = call
	s.mu.Unlock()

	state := s.driver.GetState(ctx, light)

	s.mu.Lock()
	s.cache[light.ID] = cacheEntry{state: state, at: time.Now()}
	delete(s.flight, light.ID)
	s.mu.Unlock()

	call.state = state
	close(call.done)
	return state
}

func (s *Service) cached(id string) (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[id]
	if !ok || time.Since(entry.at) >= s.ttl {
		return State{}, false
	}
	return entry.state, true
}

func (s *Service) store(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[state.ID] = cacheEntry{state: state, at: time.Now()}
}
