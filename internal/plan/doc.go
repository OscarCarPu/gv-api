// Package plan exposes endpoints for the day-planning feature.
//
// Plan blocks are time-boxed slots on a single day, either linked to a task or
// standing alone with a label. The plan never mutates tasks or time entries.
//
// Endpoints:
//
//	GET    /plan/today                 - blocks + totals + budget for today
//	POST   /plan/blocks                - create a block
//	PUT    /plan/blocks/{id}           - update a block
//	DELETE /plan/blocks/{id}           - delete a block
package plan
