// Package lights controls the house's Bluetooth bulbs (Domotics section).
//
// Endpoints (semiprivate auth, same tier as varieties):
//
//	GET  /domotics/lights          - configured bulbs, no live state
//	GET  /domotics/lights/state    - every bulb's current state
//	GET  /domotics/lights/{id}     - one bulb's current state
//	POST /domotics/lights/{id}     - apply one command, returns the resulting state
//
// # Where the Bluetooth happens
//
// Here. The API talks to the bulbs itself, through BlueZ on the host it runs on:
//
//	client -> gv-api -> BlueZ -> bulb
//
// The container needs nothing but the host's D-Bus socket bind-mounted; see gatt.go for the
// details and docker-compose.yaml for the mount. Without a working adapter every bulb simply
// reads offline with an explanation, so the API starts and runs anywhere.
//
// This replaced a Python daemon on a second machine, from back when the server had no radio.
// The hop cost a process to keep alive, a shared secret, and an outage every time that laptop
// slept.
//
// # Layout
//
//	config.go    which bulbs exist, from the LIGHTS env var
//	dto.go       the wire shapes and the command vocabulary
//	service.go   read cache, in-flight collapsing, and the settle loop
//	driver.go    per-bulb serialisation, last known values, idle disconnect
//	protocol.go  per-model frame encoding (currently the LEXMAN ZBEK-13)
//	gatt.go      BlueZ over D-Bus
//
// # Why it lives here rather than in gv-web
//
// It began in gv-web, which made the web app a backend and forced every other client to talk
// to two hosts. Lights are a plain JSON API, so they belong with the rest of the API and both
// gv-web and gv-android now reach them the same way they reach tasks or habits.
package lights
