// Package plan exposes endpoints for the day-planning feature.
//
// Plan blocks are time-boxed slots on a single day. Each block either points
// at an existing task (linked block, gives the UI ▶/✓ shortcuts that go
// through the existing task/time-entry endpoints) or stands alone with a
// label (free-time block: "comer", "thing 1"). The plan never mutates tasks
// or time entries — it is a read-only mirror of intent.
//
// Endpoints:
//
//	GET    /plan/today                 - blocks + totals + budget for today
//	POST   /plan/blocks                - create a block
//	PUT    /plan/blocks/{id}           - update a block
//	DELETE /plan/blocks/{id}           - delete a block
package plan
