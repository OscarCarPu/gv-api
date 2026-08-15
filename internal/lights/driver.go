package lights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Driver applies commands to a bulb.
//
// Implementations must not return an error for an unreachable bulb: they return a State with
// Online false and Error set, so one dead bulb cannot fail a request covering several. An
// error is reserved for the driver itself being unusable.
type Driver interface {
	Kind() string
	GetState(ctx context.Context, light Config) State
	Apply(ctx context.Context, light Config, cmd Command) State
}

// --- bridge driver -------------------------------------------------------------------

// BridgeDriver talks to the BLE bridge daemon over HTTP.
//
// A cold call is slow and legitimately so: when BlueZ has dropped an unbonded bulb's object it
// must rediscover it (~8s) before it can even connect, and a full read is three round-trips
// after that — measured at ~11s. Hence the generous default timeout; warm calls return in well
// under a second.
type BridgeDriver struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewBridgeDriver builds a driver for the daemon at baseURL. Timeout of 0 uses the default.
func NewBridgeDriver(baseURL, token string, timeout time.Duration) *BridgeDriver {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &BridgeDriver{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeout},
	}
}

func (d *BridgeDriver) Kind() string { return "bridge" }

func (d *BridgeDriver) GetState(ctx context.Context, light Config) State {
	return d.call(ctx, light, "/state", nil)
}

func (d *BridgeDriver) Apply(ctx context.Context, light Config, cmd Command) State {
	return d.call(ctx, light, "/command", &cmd)
}

// bridgeRequest is the daemon's wire format. The bridge holds no registry of its own, so
// every request carries the bulb it applies to.
type bridgeRequest struct {
	Device  bridgeDevice `json:"device"`
	Command *Command     `json:"command,omitempty"`
}

type bridgeDevice struct {
	ID       string         `json:"id"`
	Address  string         `json:"address"`
	Protocol string         `json:"protocol"`
	Options  map[string]any `json:"options"`
}

func (d *BridgeDriver) call(ctx context.Context, light Config, path string, cmd *Command) State {
	now := time.Now().UnixMilli()

	if d.baseURL == "" {
		return offlineState(light, "LIGHTS_BRIDGE_URL is not set", now)
	}

	options := light.Options
	if options == nil {
		options = map[string]any{}
	}
	body, err := json.Marshal(bridgeRequest{
		Device: bridgeDevice{
			ID:       light.ID,
			Address:  light.Address,
			Protocol: light.Protocol,
			Options:  options,
		},
		Command: cmd,
	})
	if err != nil {
		return offlineState(light, err.Error(), now)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return offlineState(light, err.Error(), now)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		// Never propagate: an unreachable bridge means every bulb reads offline, not a 500.
		return offlineState(light, bridgeErrorMessage(err), now)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		msg := fmt.Sprintf("bridge %d", resp.StatusCode)
		if len(detail) > 0 {
			msg += ": " + strings.TrimSpace(string(detail))
		}
		return offlineState(light, msg, now)
	}

	// Start from the offline baseline so any field the bridge omits has a sane value, then
	// let the bridge's answer win. Online is pre-set to true because a 200 means the bridge
	// handled the bulb: only an explicit "online": false in the body says otherwise, and a
	// bridge that simply omits the field must not read as unreachable.
	state := offlineState(light, "", now)
	state.Online = true
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return offlineState(light, "bridge sent invalid JSON: "+err.Error(), now)
	}

	// Identity and capabilities are ours, not the bridge's — it only knows the wire protocol.
	state.ID = light.ID
	state.Name = light.Name
	state.Model = light.Model
	state.SupportsColor = *light.SupportsColor
	state.SupportsColorTemp = *light.SupportsColorTemp
	state.MinColorTemp = *light.MinColorTemp
	state.MaxColorTemp = *light.MaxColorTemp
	state.UpdatedAt = now
	return state
}

func bridgeErrorMessage(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "Client.Timeout") {
		return "bridge timed out"
	}
	return msg
}

// --- mock driver ---------------------------------------------------------------------

// MockDriver keeps bulb state in memory and touches no hardware. It is the default, so the
// Domotics UI is workable in development and on any deployment without a bridge configured.
type MockDriver struct {
	mu     sync.Mutex
	states map[string]State
}

func NewMockDriver() *MockDriver {
	return &MockDriver{states: map[string]State{}}
}

func (d *MockDriver) Kind() string { return "mock" }

func (d *MockDriver) GetState(_ context.Context, light Config) State {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current(light)
}

func (d *MockDriver) Apply(_ context.Context, light Config, cmd Command) State {
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.current(light)
	switch cmd.Type {
	case CommandPower:
		state.Power = *cmd.On
	case CommandBrightness:
		state.Brightness = float64(clampInt(*cmd.Value, 0, 100))
	case CommandColor:
		state.Color = RGB{
			R: clampInt(cmd.Color.R, 0, 255),
			G: clampInt(cmd.Color.G, 0, 255),
			B: clampInt(cmd.Color.B, 0, 255),
		}
		state.Mode = "color"
	case CommandColorTemp:
		state.ColorTemp = float64(clampInt(*cmd.Kelvin, int(*light.MinColorTemp), int(*light.MaxColorTemp)))
		state.Mode = "white"
	}
	// Deliberately does not infer Power from a brightness or colour command: on the real
	// hardware those are separate frames, and a dimmed bulb that is off stays off. Guessing
	// otherwise made the UI claim "on" over a dark room.
	state.UpdatedAt = time.Now().UnixMilli()
	d.states[light.ID] = state
	return state
}

// current must be called with the lock held.
func (d *MockDriver) current(light Config) State {
	state, ok := d.states[light.ID]
	if !ok {
		state = offlineState(light, "", time.Now().UnixMilli())
		state.Brightness = 60
		state.ColorTemp = (*light.MinColorTemp + *light.MaxColorTemp) / 2
		d.states[light.ID] = state
	}
	// Identity and capabilities follow config, so an env edit shows up without clearing state.
	state.Online = true
	state.Name = light.Name
	state.Model = light.Model
	state.SupportsColor = *light.SupportsColor
	state.SupportsColorTemp = *light.SupportsColorTemp
	state.MinColorTemp = *light.MinColorTemp
	state.MaxColorTemp = *light.MaxColorTemp
	state.Error = ""
	return state
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
