# Accepted parity deviations

Differences between the Go release (`go-final`) and the Python service that are intended. A
corpus entry with `accepted: <id>` is checked against its `expect_py` instead of against Go,
and `<id>` must be one of the headings below.

## lights-removed

The lights domain (`/domotics/lights/*`) is retired with the rewrite (spec `api-surface`): the
bulbs are driven from gv-web's BLE bridge. Every lights route answers the plain 404 of an
unknown path. The `lights` table is kept until a later change drops it.
