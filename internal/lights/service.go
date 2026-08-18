package lights

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned for an unknown bulb id; the handler maps it to 404.
var ErrNotFound = errors.New("light not found")

// How many slugs to try before giving up on naming a new bulb: "kitchen", "kitchen-2", ...
const slugAttempts = 25

// Service reads and writes bulbs through a Driver, with a short read cache: BLE reads take
// hundreds of milliseconds and two overlapping reads of one bulb tend to fail both, so reads
// inside the TTL come from memory and concurrent reads share one in-flight call. Writes
// bypass the cache and replace it with their result.
type Service struct {
	repo   Repository
	driver Driver
	ttl    time.Duration

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

func NewService(repo Repository, driver Driver, ttl time.Duration, settleAttempts int, settleDelay time.Duration) *Service {
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
		repo:           repo,
		driver:         driver,
		ttl:            ttl,
		settleAttempts: settleAttempts,
		settleDelay:    settleDelay,
		cache:          map[string]cacheEntry{},
		flight:         map[string]*inflight{},
	}
}

func (s *Service) List(ctx context.Context) ([]PublicLight, error) {
	lights, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return publicLights(lights), nil
}

// States reads every bulb in parallel — each is an independent connection, and serialising
// them would multiply the worst case by the number of bulbs.
func (s *Service) States(ctx context.Context, force bool) ([]State, error) {
	lights, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]State, len(lights))

	var wg sync.WaitGroup
	for i, light := range lights {
		wg.Add(1)
		go func(i int, light Light) {
			defer wg.Done()
			out[i] = s.read(ctx, light, force)
		}(i, light)
	}
	wg.Wait()
	return out, nil
}

func (s *Service) State(ctx context.Context, id string, force bool) (State, error) {
	light, err := s.repo.Get(ctx, id)
	if err != nil {
		return State{}, err
	}
	return s.read(ctx, light, force), nil
}

func (s *Service) Send(ctx context.Context, id string, cmd Command) (State, error) {
	light, err := s.repo.Get(ctx, id)
	if err != nil {
		return State{}, err
	}
	if err := cmd.Validate(); err != nil {
		return State{}, err
	}

	state := s.driver.Apply(ctx, light, cmd)
	state = s.settle(ctx, light, cmd, state)
	s.store(state)
	return state, nil
}

// settle re-applies a command until the bulb holds the requested value: these bulbs
// sometimes land on a neighbouring step and stay there, and fixing that in each client
// would mean three copies of the same retry. Only continuous values are settled — power
// either took or did not, and a bulb that cannot be read back has nothing to compare.
func (s *Service) settle(ctx context.Context, light Light, cmd Command, state State) State {
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

func (s *Service) read(ctx context.Context, light Light, force bool) State {
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

// --- managing which bulbs exist -------------------------------------------------------

// Create registers a bulb picked off a scan. The caller gives a name, an address and a
// model; the rest defaults to what that model can do, because nobody adding a lamp to a
// bedroom knows its kelvin range.
func (s *Service) Create(ctx context.Context, req CreateLightRequest) (PublicLight, error) {
	if err := req.Validate(); err != nil {
		return PublicLight{}, err
	}
	proto, _ := protocolFor(req.Protocol) // Validate already rejected an unknown one
	info := proto.Info()

	light := Light{
		Name:              strings.TrimSpace(req.Name),
		Model:             valueOr(req.Model, info.Label),
		Address:           normalizeAddress(req.Address),
		Protocol:          req.Protocol,
		SupportsColor:     valueOr(req.SupportsColor, info.SupportsColor),
		SupportsColorTemp: valueOr(req.SupportsColorTemp, info.SupportsColorTemp),
		MinColorTemp:      valueOr(req.MinColorTemp, info.MinColorTemp),
		MaxColorTemp:      valueOr(req.MaxColorTemp, info.MaxColorTemp),
		Options:           req.Options,
	}

	// Two bulbs called "Bedroom lamp" are perfectly reasonable; two rows with the same id are
	// not. Suffix until one sticks rather than making the person rename their lamp.
	base := slugify(light.Name)
	for attempt := 1; attempt <= slugAttempts; attempt++ {
		light.ID = base
		if attempt > 1 {
			light.ID = fmt.Sprintf("%s-%d", base, attempt)
		}
		created, err := s.repo.Create(ctx, light)
		switch {
		case err == nil:
			slog.Info("light added", "light", created.ID, "protocol", created.Protocol)
			return created.Public(), nil
		case errors.Is(err, errDuplicateID):
			continue
		default:
			return PublicLight{}, err
		}
	}
	return PublicLight{}, fmt.Errorf("could not find a free id for %q", light.Name)
}

// Update edits a registered bulb. Omitted fields keep what they had.
func (s *Service) Update(ctx context.Context, id string, req UpdateLightRequest) (PublicLight, error) {
	if err := req.Validate(); err != nil {
		return PublicLight{}, err
	}
	light, err := s.repo.Get(ctx, id)
	if err != nil {
		return PublicLight{}, err
	}

	light.Name = strings.TrimSpace(valueOr(req.Name, light.Name))
	light.Model = valueOr(req.Model, light.Model)
	light.Protocol = valueOr(req.Protocol, light.Protocol)
	light.SupportsColor = valueOr(req.SupportsColor, light.SupportsColor)
	light.SupportsColorTemp = valueOr(req.SupportsColorTemp, light.SupportsColorTemp)
	light.MinColorTemp = valueOr(req.MinColorTemp, light.MinColorTemp)
	light.MaxColorTemp = valueOr(req.MaxColorTemp, light.MaxColorTemp)
	if req.Options != nil {
		light.Options = req.Options
	}
	if light.MinColorTemp >= light.MaxColorTemp {
		return PublicLight{}, fmt.Errorf(`%w: "minColorTemp" must be below "maxColorTemp"`, ErrInvalidCommand)
	}

	updated, err := s.repo.Update(ctx, light)
	if err != nil {
		return PublicLight{}, err
	}
	// A renamed or reconfigured bulb must not keep answering from a cache built under the old
	// one — the name travels inside State.
	s.forget(id)
	return updated.Public(), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.forget(id)
	slog.Info("light removed", "light", id)
	return nil
}

// Discover lists bulbs in range and marks the ones already registered, so the add screen
// does not offer a lamp that is already on the page.
func (s *Service) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	found, err := s.driver.Discover(ctx, window)
	if err != nil {
		return nil, err
	}
	lights, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	registered := make(map[string]bool, len(lights))
	for _, light := range lights {
		registered[normalizeAddress(light.Address)] = true
	}
	for i, device := range found {
		found[i].Known = registered[normalizeAddress(device.Address)]
	}
	return found, nil
}

func (s *Service) Protocols() []ProtocolInfo { return protocolInfos() }

// forget drops a bulb's cached state, for when the bulb it described has changed or gone.
func (s *Service) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, id)
}

// valueOr resolves an optional field against a fallback.
func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func (s *Service) store(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[state.ID] = cacheEntry{state: state, at: time.Now()}
}
