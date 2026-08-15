// Package lights controls the house's Bluetooth bulbs (Domotics section).
//
// Endpoints (semiprivate auth, same tier as varieties):
//
//	GET  /domotics/lights          - configured bulbs, no live state
//	GET  /domotics/lights/state    - every bulb's current state
//	GET  /domotics/lights/{id}     - one bulb's current state
//	POST /domotics/lights/{id}     - apply one command, returns the resulting state
//
// # Where the Bluetooth actually happens
//
// Nowhere near this process. The server has no Bluetooth radio at all, and even with a
// dongle, speaking BLE from a container would mean handing it the host's D-Bus and network
// namespace. So the radio work lives in a small daemon (gv-web's scripts/ble-bridge) running
// on a LAN machine that does have one, and this package talks to it over HTTP:
//
//	client -> gv-api -> bridge daemon -> bulb
//
// The bridge is the only thing that knows a bulb's wire protocol. This package knows which
// bulbs exist (from the LIGHTS env var), enforces the command vocabulary, and caches reads.
//
// # Why it lives here rather than in gv-web
//
// It began in gv-web, which made the web app a backend and forced every other client to talk
// to two hosts. Lights are a plain JSON API, so they belong with the rest of the API and both
// gv-web and gv-android now reach them the same way they reach tasks or habits.
package lights
