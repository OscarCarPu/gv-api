/*
Package calendar mirrors the user's Google calendars and lets them be edited from here.

Google stays the source of truth. Reads are served from the local copy, which is kept current
by incremental sync; writes go to Google first and are stored only once it accepts them, so a
row here always corresponds to an event over there.

Shape:

	account   one Google login, one OAuth grant, tokens encrypted at rest
	calendar  one entry of that account's calendar list, one sync cursor, one push channel
	event     one mirrored event: a one-off, a series master, or an override of one occurrence

Recurring series are stored the way Google stores them — a master carrying the RRULE plus a row
per modified or cancelled occurrence — and expanded on read, in the event's own zone. Storing
the expansion would mean either unbounded rows for an endless series or a horizon that quietly
truncates the calendar.

Freshness comes from two mechanisms, and both are needed: push notifications make a change in
Google show up here in seconds, and the poll is the safety net for the notifications Google
admits it drops and for channels that expire without being renewed.

Endpoints (full auth):

	GET    /calendar/accounts                 - connected accounts and their state
	POST   /calendar/accounts/auth-url        - consent url for adding an account
	PATCH  /calendar/accounts/{id}            - label and colour
	DELETE /calendar/accounts/{id}            - stop channels, revoke the grant, drop the data
	POST   /calendar/accounts/{id}/resync     - throw the local copy away and rebuild it
	GET    /calendar/calendars                - calendars, grouped per account
	PATCH  /calendar/calendars/{id}           - sync on/off, visibility, colour override
	GET    /calendar/events?from&to           - expanded occurrences in a range
	POST   /calendar/events                   - create in Google, then mirror
	GET    /calendar/events/{ref}             - one event or one occurrence
	PATCH  /calendar/events/{ref}             - patch, scope=all|instance|following
	DELETE /calendar/events/{ref}             - delete, same scopes
	POST   /calendar/events/{ref}/move        - change calendar (recreates across accounts)
	POST   /calendar/sync                     - sync now
	GET    /calendar/sync/status              - cursors, channels, recent runs, errors
	GET    /calendar/stream                   - SSE: something changed, refetch

Endpoints (public, they cannot carry a bearer token):

	GET    /calendar/google/callback          - OAuth redirect, guarded by a signed state
	POST   /calendar/google/webhook           - push notification, guarded by a channel token

{ref} is an event id ("12") or one occurrence of a series ("12@2026-08-20T07:00:00Z", the
original start of the occurrence).
*/
package calendar
