/*
Package calendar holds events: a name and a time range. gv is the only calendar interface.

An event can be linked from a plan block through plan_blocks.event_ref, which is the event id as
a string. Moving an event moves the linked block; deleting it detaches the block.

Endpoints (full auth):

	GET    /calendar/events?from&to   - events overlapping the range
	POST   /calendar/events           - create
	GET    /calendar/events/{id}      - one event
	PATCH  /calendar/events/{id}      - name and/or times
	DELETE /calendar/events/{id}      - delete
*/
package calendar
