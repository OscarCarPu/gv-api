# Lights (Domotics)

Control for the house's Bluetooth bulbs. **Semiprivate auth** — either the full or the
semiprivate token gets in, same tier as varieties, because this is house control rather than
personal data.

## Where the Bluetooth happens

Here. The API talks to the bulbs itself, over BlueZ on the host it runs on:

```
client → gv-api → BlueZ → bulb
```

The container needs one thing: the host's D-Bus socket, bind-mounted (see
`docker-compose.yaml`). No host networking, no extra capabilities, no root — BlueZ's default
policy lets any local uid send to `org.bluez`. Without the mount the API still runs and every
bulb reads `online: false`.

This replaced a Python daemon on a second machine, from back when the server had no radio. The
hop cost a process to keep alive, a shared secret, and an outage every time that laptop slept.

## Which bulbs exist

A table, managed from the Lights tab: scan for what is in range, pick one, give it a name. The
id is a slug of that name (`bedroom`, `bedroom-2`), assigned once and kept through renames, so
a client holding state under it never has the ground move.

BLE addresses stay server-side. The one exception is `/discover`, where the address is the
only handle a person has for telling two nameless lamps apart.

## Endpoints

### `GET /domotics/lights`

Configured bulbs and their capabilities. Touches no hardware, so it is always fast.

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
      "updatedAt": 1786800215474
    }
  ]
}
```

`brightness` is 0-100, normalised from whatever scale the bulb uses. Field names are camelCase
rather than this API's usual snake_case, which is what three clients already model.

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

`400` for a malformed command, `404` for an unknown bulb.

**A write is verified and corrected.** These bulbs do not always land where they are told —
a value arrives late, or the lamp settles on a neighbouring step and stays there. Rather than
leave each client to notice and nudge it, the API closes the loop: it writes, waits
`LIGHTS_SETTLE_DELAY_MS` for the lamp to transition, reads back, and re-applies if the value
drifted, up to `LIGHTS_SETTLE_ATTEMPTS` times. The state you get back is therefore what the
bulb actually holds, not what was requested.

Only brightness and colour temperature are settled — power is a boolean with no "near enough"
to chase, so it costs no extra read. A bulb that goes offline mid-correction is reported as
offline rather than as the value we hoped for.

**Commands are independent.** Setting brightness or colour does *not* switch a bulb on: on the
real hardware those are separate frames, so dimming a bulb that is off only changes how it will
look when switched on. Do not infer power from them — a client that did reported "on" over a
dark room, and turned its "All on" button into a no-op.

## Errors are per-bulb

`/state` covers several bulbs, so a single unreachable one must not fail the request. Drivers
never return an error for an unreachable bulb; they return `online: false` with `error` set,
inside a `200`. A missing adapter therefore degrades to "everything offline", not a 500.

The message is deliberately vague about the cause ("bulb not found — is it powered and in
range?"). BlueZ's own errors embed the D-Bus object path, which contains the bulb's MAC; that
is meaningless to a person and needless exposure of the house's hardware, so it stays in the
log.

`/discover` is the exception: it fails with `503` rather than an empty list, because "no bulbs
here" and "this host cannot look" are not the same answer.

## Adding and removing bulbs

### `GET /domotics/lights/discover?seconds=8`

Scans for bulbs in range and holds the request open for the length of the scan — the answer
does not exist until the radio has been listening for a while. Capped at 30s.

```json
{
  "devices": [
    { "address": "08:6B:D7:F6:B0:D0", "name": "ZBEK-13", "rssi": -47, "known": false }
  ]
}
```

Strongest signal first: the bulb someone is standing next to is the one they mean. `known` is
true for an address already registered, so the UI can show it as added rather than offering it
twice. `503` when this host has no Bluetooth.

A bulb the vendor app is connected to on a phone will **not** appear — these lamps take one
central at a time and stop advertising while another holds them.

### `GET /domotics/lights/protocols`

The bulb families this API can drive, with what each model can do. The add form uses it to
offer a model and prefill capabilities, so nobody has to know a lamp's kelvin range.

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

Adds a bulb. `name`, `address` and `protocol` are required; the capability fields are optional
overrides of what the protocol already says the model can do.

```json
{ "name": "Bedroom", "address": "08:6B:D7:F6:B0:D0", "protocol": "lexman" }
```

`201` with the new bulb. `400` for a missing field or an unknown protocol, `409` when that
address is already registered — two cards for one lamp would fight over a radio link that
takes one conversation at a time.

### `PATCH /domotics/lights/{id}`

Edits name, model, protocol or capabilities. Omitted fields keep their value. The address is
not editable: a different address is a different lamp.

### `DELETE /domotics/lights/{id}`

`204`, or `404` if it was already gone.

## Configuration

```
LIGHTS_DRIVER=mock|bluez           # mock (default) touches no hardware
LIGHTS_ADAPTER=hci0
LIGHTS_CONNECT_TIMEOUT_MS=20000    # a cold connect measures ~11s; don't go much below
LIGHTS_CACHE_MS=2000
LIGHTS_IDLE_DISCONNECT_MS=90000    # hand an unused bulb back to its own remote
LIGHTS_SETTLE_ATTEMPTS=2           # re-apply a drifting write this many times (0 = off)
LIGHTS_SETTLE_DELAY_MS=400         # let the lamp transition before checking
```

The mock driver answers `/discover` too, with invented bulbs, so the whole add flow is
developable on a machine with no radio.

**If every bulb reads offline on a host that does have Bluetooth**, check the container's uid
before anything else. The reference `dbus-daemon` resolves the connecting uid to a host user
and resets the connection when it cannot; this image runs as uid 100, which typically exists
on no host. The symptom is `cannot reach the system bus` in the log with the socket plainly
mounted, and the fix is a `user:` line naming a uid the host knows. Running the API outside
Docker does not hit this, because your own uid is real.

**Idle disconnect is worth understanding.** These lamps accept one central, so holding a link
forever locks out their own remote and the vendor app. A bulb nobody has touched for
`LIGHTS_IDLE_DISCONNECT_MS` is released. The flip side: any client polling faster than that
keeps its bulbs connected, which is why the wall remote stops working while the Lights tab is
open.

Adding a bulb family means implementing `protocol` in `internal/lights/protocol.go` — the
frames for on/off, brightness and colour, and optionally a readback. Use any BLE scanner
(`bluetoothctl scan le`, then `gatt list-attributes`) to find the write characteristic: it is
almost always the single write handle on a vendor service.
