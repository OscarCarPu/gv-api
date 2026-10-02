// Package uptime reports how much of the time the home lab and its ESP32 watchdog have
// been reachable.
//
// It owns no data: it reads central-pipeline's dbt marts over a separate read-only DSN.
//
// Endpoints:
//
//	GET /domotics/uptime          - current state per device plus the four precomputed ranges
//	GET /domotics/uptime/windows  - state changes over an arbitrary range, with its own percentage
package uptime
