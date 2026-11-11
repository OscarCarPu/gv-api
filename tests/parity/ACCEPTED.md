# Accepted parity deviations

Differences between the Go release (`go-final`) and the Python service that are intended. A
corpus entry with `accepted: <id>` is checked against its `expect_py` instead of against Go,
and `<id>` must be one of the headings below.

## lights-removed

The lights domain (`/domotics/lights/*`) is retired with the rewrite (spec `api-surface`): the
bulbs are driven from gv-web's BLE bridge. Every lights route answers the 404 of an
unknown path. The `lights` table is kept until a later change drops it.

## name-length-in-characters

Name limits (habits 40, rutas 200, tasks 40, plan labels 200) count characters, as the `varchar` columns do. Go
counted UTF-8 bytes, so a 21-letter name of `ñ` (42 bytes) was a 400 there and is accepted now.
No corpus entry: it would create a row on one side only.

## error-text-unescaped

Validation messages quote the client's value as sent. Go escaped quotes, backslashes and control
characters in it (`"2026\"06"`).

## real-seven-digits

`real` values (habit targets, logs, history) are written with 7 significant digits. Go printed
the shortest decimal that reads back as the same float32; they differ only on values with more
than 7 significant digits (Go `0.85714287`, now `0.8571429`). The runner compares floats at 7
significant digits.

## history-start-clamped

Without `start_at`, history starts one month (daily) or one year (monthly) back, clamped to the
end of a shorter month: on Mar 31 it starts on Feb 28. Go rolled the overflow forward (Mar 3).
On the 29th–31st the default-range history entries in `habits.yaml` show this difference.

## cors-starlette

CORS uses Starlette's middleware with the same origins, methods, headers, exposed headers and
300 s cache. Its preflights list every allowed method and header instead of echoing the
requested ones, and a refused preflight (foreign origin, method or header) is a 400 instead of
a 200 without CORS headers. The browser outcome is the same.

## framework-errors

Unknown paths and methods get FastAPI's answers: 404 `{"detail": "Not Found"}` and 405
`{"detail": "Method Not Allowed"}` with one comma-separated `Allow` header. The runner checks the
status and the allowed methods of these replies, not their bodies.

## strict-json

A body with content after the JSON value is malformed (400). Go read the first value and
ignored the rest.

## jwt-hs256-only

Only HS256 tokens with an `exp` claim are accepted, and PyJWT's `iat` and `aud` checks apply.
Every token either service issued is HS256 with `exp`, so none is affected.

## unhandled-500-text

An unhandled failure answers Starlette's `Internal Server Error` text instead of an empty body.

## always-present-fields

Every field of a response is always sent, as `null` or `[]` when empty. Go left some keys out
when empty (`omitempty`: tree nodes, project children, `recurrence`). The runner drops a
`null` or `[]` that Python sends where Go sent no key. gv-web reads these fields with `?.` or
`??`, so both forms work.

## root-projects-full-row

`GET /tasks/projects` returns each project's real `started_at`, `finished_at` and `priority`.
Go left them `null` and `0` because it did not copy them from the row.

## plan-block-times-required

`POST /plan/blocks` without `started_at` or `ended_at` is a 400 `ended_at must be after
started_at`. Go read a missing time as year 1, so a block with only `ended_at` was created on
0001-01-01.

## calendar-iso-instants

`/calendar/events` filters read an instant with `datetime.fromisoformat`, which also takes ISO
forms Go's RFC 3339 parser refused (no seconds, a space instead of `T`). It still needs an
offset, and every value Go accepted means the same instant.

## plan-date-local-on-update

Moving a plan block's start sets its `plan_date` to the start's local date (`TIMEZONE`), as
creating it does. Go used the UTC date on update, so a block moved to 00:00-02:00 Madrid time
was listed under the previous day.
