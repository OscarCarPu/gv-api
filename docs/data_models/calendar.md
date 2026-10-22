# Calendar data model

### calendar_events

| Column | Type | Notes |
|---|---|---|
| `id` | serial | Referenced by `plan_blocks.event_ref` as a string. |
| `name` | text | |
| `start_time` | timestamptz | |
| `end_time` | timestamptz | After `start_time`. |
| `created_at`, `updated_at` | timestamptz | |

Indexed on `(start_time, end_time)`.
