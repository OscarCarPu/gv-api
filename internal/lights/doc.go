// Package lights controls the house's Bluetooth bulbs (Domotics section).
//
// Endpoints (semiprivate auth):
//
//	GET    /domotics/lights          - registered bulbs, no live state
//	GET    /domotics/lights/state    - every bulb's current state
//	GET    /domotics/lights/discover - bulbs in range, marking the known ones
//	GET    /domotics/lights/protocols - supported bulb protocols
//	POST   /domotics/lights          - register a bulb
//	GET    /domotics/lights/{id}     - one bulb's current state
//	POST   /domotics/lights/{id}     - apply one command, returns the new state
//	PATCH  /domotics/lights/{id}     - edit a registered bulb
//	DELETE /domotics/lights/{id}     - unregister a bulb
//
// The API talks to the bulbs itself, through BlueZ on the host it runs on:
// client -> gv-api -> BlueZ -> bulb. The container needs only the host's D-Bus
// socket bind-mounted (see gatt.go and docker-compose.yaml). Without a working
// adapter every bulb reads offline with an explanation, so the API runs anywhere.
//
// Layout:
//
//	dto.go         wire shapes and the command vocabulary
//	light.go       the bulb record
//	repository.go  which bulbs exist (a table)
//	service.go     read cache, in-flight collapsing, settle loop
//	driver.go      per-bulb serialisation, last known values, idle disconnect
//	protocol.go    per-model frame encoding (currently the LEXMAN ZBEK-13)
//	gatt.go        BlueZ over D-Bus
package lights
