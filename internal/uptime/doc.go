// Package uptime reports how much of the time the home lab and its ESP32 watchdog have
// been reachable.
//
// It is the one domain that owns no data. The events come from an MQTT topic each device
// publishes to, and central-pipeline turns them into two tables in its own PostgreSQL
// instance: one window per state change, plus four precomputed percentages per device.
// gv-api only reads them, over a second pool with a DSN of its own, because that schema
// belongs to dbt — which drops and recreates it on every run — and not to gv's
// migrations.
//
// Endpoints:
//
//	GET /domotics/uptime          - current state per device plus the four precomputed ranges
//	GET /domotics/uptime/windows  - state changes over an arbitrary range, with its own percentage
package uptime
