# Accepted parity deviations

Differences between the Go release (`go-final`) and the Python service that are intended. A
corpus entry with `accepted: <id>` is checked against its `expect_py` instead of against Go,
and `<id>` must be one of the headings below.

## lights-removed

The lights domain (`/domotics/lights/*`) is retired with the rewrite (spec `api-surface`): the
bulbs are driven from gv-web's BLE bridge. Every lights route answers the plain 404 of an
unknown path. The `lights` table is kept until a later change drops it.

## name-length-in-characters

Name limits (habits 40, rutas 200, tasks 40) count characters, as the `varchar` columns do. Go
counted UTF-8 bytes, so a 21-letter name of `ñ` (42 bytes) was a 400 there and is accepted now.
No corpus entry: it would create a row on one side only.

## error-text-unescaped

Validation messages quote the client's value as sent. Go escaped quotes, backslashes and control
characters in it (`"2026\"06"`).

## real-seven-digits

`real` values (habit targets, logs, history) are written with 7 significant digits. Go printed
the shortest decimal that reads back as the same float32; they differ only on values with more
than 7 significant digits (Go `123456.78`, now `123456.8`).

## history-start-clamped

Without `start_at`, history starts one month (daily) or one year (monthly) back, clamped to the
end of a shorter month: on Mar 31 it starts on Feb 28. Go rolled the overflow forward (Mar 3).
On the 29th–31st the default-range history entries in `habits.yaml` show this difference.

## cors-starlette

CORS uses Starlette's middleware with the same origins, methods, headers, exposed headers and
300 s cache. Its preflights list every allowed method and header instead of echoing the
requested ones, and a refused preflight (foreign origin, method or header) is a 400 instead of
a 200 without CORS headers. The browser outcome is the same.
