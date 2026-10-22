// Package uptime reports how much of the time the service answered, from the outages table.
//
// The table is a timeline of ranges (up, down, failover). A heartbeat keeps the last up range
// current, and a failover record adds the downtime and failover ranges (see internal/failover).
//
// Endpoints:
//
//	GET /domotics/uptime          - up since, plus the four lookbacks
//	GET /domotics/uptime/windows  - the ranges over an arbitrary range, with its own percentage
package uptime
