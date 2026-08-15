# Lights (Domotics)

Control for the house's Bluetooth bulbs. **Semiprivate auth** — either the full or the
semiprivate token gets in, same tier as varieties, because this is house control rather than
personal data.

## Where the Bluetooth happens

Not in this process. The server has no Bluetooth radio at all, and even with a dongle,
speaking BLE from a container would mean handing it the host's D-Bus and network namespace.
The radio work lives in a small daemon — `gv-web/scripts/ble-bridge/` — running on a LAN
machine that does have one:

```
client → gv-api → bridge daemon → bulb
```

The bridge is the only thing that knows a bulb's wire protocol. This API knows which bulbs
exist, enforces the command vocabulary, and caches reads.

**Consequence worth knowing:** the Lights section only works while the bridge host is up. When
it is not, every bulb comes back `online: false` with an `error` — never a failed request.

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
rather than this API's usual snake_case: the shape is passed through from the bridge unchanged.

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
inside a `200`. A dead bridge therefore degrades to "everything offline", not a 500.

## Configuration

```
LIGHTS_DRIVER=mock|bridge          # mock (default) touches no hardware
LIGHTS_BRIDGE_URL=http://host:8477
LIGHTS_BRIDGE_TOKEN=<shared secret>
LIGHTS_BRIDGE_TIMEOUT_MS=20000     # a cold BLE connect measures ~11s; don't go much below
LIGHTS_CACHE_MS=2000
LIGHTS_SETTLE_ATTEMPTS=2           # re-apply a drifting write this many times (0 = off)
LIGHTS_SETTLE_DELAY_MS=400         # let the lamp transition before checking
LIGHTS=[{"id":"bedroom","address":"AA:BB:..","protocol":"lexman", ...}]
```

`LIGHTS` is a JSON array, so adding a bulb is an env change and a restart. A malformed entry is
skipped with a warning rather than failing startup — one bad bulb should not take the API down.

Supported `protocol` values are the ones implemented in the bridge; see
`gv-web/scripts/ble-bridge/README.md` for the current list and for how to add a model.
