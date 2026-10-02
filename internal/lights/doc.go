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
//	                                   ({"type":"crazy","on":true} starts the sweep)
//	PATCH  /domotics/lights/{id}     - edit a registered bulb
//	DELETE /domotics/lights/{id}     - unregister a bulb
//
// The API drives the bulbs through BlueZ over the host's D-Bus socket. Without a working
// adapter every bulb reads offline, so the API still runs anywhere.
package lights
