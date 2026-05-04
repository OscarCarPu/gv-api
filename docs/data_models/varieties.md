# Weed Varieties - Data Models

## Tables

### weed_varieties

| Column   | Type    | Constraints                                                                              |
|----------|---------|------------------------------------------------------------------------------------------|
| id       | SERIAL  | PRIMARY KEY                                                                              |
| name     | TEXT    | NOT NULL                                                                                 |
| scent    | REAL    | NOT NULL, CHECK (`0 <= scent <= 10`)                                                     |
| flavor   | REAL    | NOT NULL, CHECK (`0 <= flavor <= 10`)                                                    |
| power    | REAL    | NOT NULL, CHECK (`0 <= power <= 10`)                                                     |
| quality  | REAL    | NOT NULL, CHECK (`0 <= quality <= 10`)                                                   |
| score    | REAL    | NOT NULL, GENERATED ALWAYS AS `(scent + flavor + power + quality) / 4.0` STORED          |
| price    | REAL    | NOT NULL                                                                                 |
| comments | TEXT    | nullable                                                                                 |

**Indexes:**
- `idx_weed_varieties_score_price` on `(score DESC, price ASC)` — backs the default list ordering.

## Notes

- `score` is a stored generated column: it is recomputed by Postgres on every insert/update of the four sensory fields and cannot be written directly.
- `price` has no min/max — negative or zero values are allowed at the schema level.
- The table has no foreign keys.
