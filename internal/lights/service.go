package lights

import (
	"context"
	"errors"
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

func NewService(registry *Registry, driver Driver, ttl time.Duration) *Service {
	if ttl < 0 {
		ttl = 0
	}
	return &Service{
		registry: registry,
		driver:   driver,
		ttl:      ttl,
		cache:    map[string]cacheEntry{},
		flight:   map[string]*inflight{},
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
	s.store(state)
	return state, nil
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
