# Lights (Domotics)

Control for the house's Bluetooth bulbs. **Semiprivate auth** (full or semiprivate token).

## How it connects

The API drives the bulbs through BlueZ on its own host:

```
client → gv-api → BlueZ → bulb
```

The container only needs the host's D-Bus socket bind-mounted (see `docker-compose.yaml`); no host networking or extra capabilities. Without it the API still runs and every bulb reads `online: false`.

## Which bulbs exist

A table, managed from the Lights tab (scan, pick, name). The id is a slug of the name (`bedroom`, `bedroom-2`), assigned once and kept through renames.

BLE addresses stay server-side, except in `/discover`, where they are the only way to tell nameless lamps apart.

## Endpoints

### `GET /domotics/lights`

Registered bulbs and their capabilities, without touching hardware or exposing addresses.

```json
[
  {
    "id": "bedroom",
    "name": "Bedroom",
    "model": "LEXMAN ZBEK-13 CCT",
    "supportsColor": false,
    "supportsColorTemp": true,
    "minColorTemp": 2700,
    "maxColorTemp": 6500
  }
]
```

BLE addresses are never included — they identify hardware in the house.

### `GET /domotics/lights/state`

Every bulb's current state. `?force=1` skips the read cache.

```json
{
  "states": [
    {
      "id": "bedroom",
      "name": "Bedroom",
      "model": "LEXMAN ZBEK-13 CCT",
      "online": true,
      "power": true,
      "brightness": 95,
      "mode": "white",
      "color": { "r": 255, "g": 255, "b": 255 },
      "colorTemp": 2700,
      "supportsColor": false,
      "supportsColorTemp": true,
      "minColorTemp": 2700,
      "maxColorTemp": 6500,
      "updatedAt": 1786800215474,
      "crazy": false
    }
  ]
}
```

`brightness` is 0-100. Fields are camelCase, unlike the rest of the API.

### `GET /domotics/lights/{id}`

One bulb. `?force=1` skips the cache. `404` for an unknown id.

### `POST /domotics/lights/{id}`

Apply one command; the response is the resulting state.

| Body | Meaning |
| --- | --- |
| `{"type":"power","on":true}` | switch on/off |
| `{"type":"brightness","value":0-100}` | set brightness |
| `{"type":"color","color":{"r":0-255,"g":..,"b":..}}` | set an RGB colour |
| `{"type":"colorTemp","kelvin":2700}` | set colour temperature |
| `{"type":"crazy","on":true}` | start or stop crazy mode |

`400` for a malformed command, `404` for an unknown bulb.

**Writes are verified.** These bulbs sometimes land on a neighbouring step, so the API waits `LIGHTS_SETTLE_DELAY_MS`, reads back and re-applies up to `LIGHTS_SETTLE_ATTEMPTS` times. The response is what the bulb actually holds.

Only brightness and colour temperature are settled. A bulb lost mid-correction is reported offline.

**Commands are independent.** Brightness and colour do not switch a bulb on; they are separate frames on the hardware. Clients must not infer power from them.

**Crazy mode.** `{"type":"crazy","on":true}` switches the bulb on and sweeps brightness 100 → 1 → 100 every 5 s and colour temperature across its range every 4 s. It runs server-side (`state.crazy`) and ends on any other command for the bulb, `{"type":"crazy","on":false}`, editing or deleting the bulb, or five failed frames in a row. While it runs, `GET /state` returns the last frame instead of querying the bulb. Frames go out every 600 ms; faster sweeps crashed a bulb until power-cycled.

## Errors are per-bulb

An unreachable bulb is `online: false` with `error` set, inside a `200`, so one dead bulb (or no adapter) never fails the request.

Messages are generic ("bulb not found — is it powered and in range?"): BlueZ errors embed the bulb's MAC, so they stay in the log.

`/discover` is the exception: it answers `503` rather than an empty list when the host cannot scan.

## Adding and removing bulbs

### `GET /domotics/lights/discover?seconds=8`

Scans for bulbs in range, holding the request open for the scan (capped at 30s).

```json
{
  "devices": [
    { "address": "08:6B:D7:F6:B0:D0", "name": "ZBEK-13", "rssi": -47, "known": false }
  ]
}
```

Strongest signal first. `known` marks addresses already registered. `503` when the host has no Bluetooth.

A bulb held by the vendor phone app will not appear: these lamps take one connection at a time and stop advertising.

### `GET /domotics/lights/protocols`

Supported bulb families and their capabilities, used by the add form to prefill them.

```json
[
  {
    "name": "lexman",
    "label": "LEXMAN / Adeo ZBEK-13 (tunable white)",
    "supportsColor": false,
    "supportsColorTemp": true,
    "minColorTemp": 2700,
    "maxColorTemp": 6500
  }
]
```

### `POST /domotics/lights`

Adds a bulb. `name`, `address` and `protocol` are required; capability fields override the protocol's defaults.

```json
{ "name": "Bedroom", "address": "08:6B:D7:F6:B0:D0", "protocol": "lexman" }
```

`201` with the new bulb. `400` for a missing field or unknown protocol, `409` if the address is already registered.

### `PATCH /domotics/lights/{id}`

Edits name, model, protocol or capabilities; omitted fields keep their value. The address cannot change.

### `DELETE /domotics/lights/{id}`

`204`, or `404` if it was already gone.

## On the backup server

With `FAILOVER_SIDE=aws` every endpoint here answers `503` before reaching the handler (after auth, except for signed media URLs):

```json
{"error": "Not available while running on the backup server. Back when home is restored.", "code": "unavailable_on_failover"}
```

## Configuration

```
LIGHTS_DRIVER=mock|bluez           # mock (default) touches no hardware
LIGHTS_ADAPTER=hci0
LIGHTS_CONNECT_TIMEOUT_MS=20000    # a cold connect measures ~11s; don't go much below
LIGHTS_CACHE_MS=2000
LIGHTS_POLL_MS=60000               # background status check, so reads are instant (0 = off)
LIGHTS_IDLE_DISCONNECT_MS=90000    # hand an unused bulb back to its own remote
LIGHTS_SETTLE_ATTEMPTS=2           # re-apply a drifting write this many times (0 = off)
LIGHTS_SETTLE_DELAY_MS=400         # let the lamp transition before checking
```

The mock driver also answers `/discover` with invented bulbs, so the add flow works without a radio.

**Every bulb offline on a host with Bluetooth?** Check the container's uid: `dbus-daemon` resets connections from uids unknown on the host (the image's default 100 usually is). The log says `cannot reach the system bus`; the fix is a `user:` line with a real host uid.

**Idle disconnect.** These lamps accept one connection, so a bulb untouched for `LIGHTS_IDLE_DISCONNECT_MS` is released for its remote and app.

**Background polling.** Every `LIGHTS_POLL_MS` each bulb is read, so `GET /state` answers from memory (trusted for two intervals; `?force=1` bypasses it). Polls do not count as use, so the bulb is released right after.

New bulb families implement `protocol` in `internal/lights/protocol.go`. Find the write characteristic with `bluetoothctl scan le` and `gatt list-attributes`.
