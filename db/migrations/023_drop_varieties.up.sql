-- The weed varieties ranking is gone from the product, so its two tables go with it.
-- Dropping is deliberate rather than a rename-and-keep: the data was a personal ranking of
-- no use to anything else in the schema, and the audit history only ever existed to explain
-- edits to that ranking.
--
-- The trigger and its function are dropped explicitly before the tables. DROP TABLE would
-- take the trigger anyway, but the function lives at schema level and would otherwise be
-- left behind referencing a %ROWTYPE that no longer exists.
DROP TRIGGER IF EXISTS weed_varieties_audit ON weed_varieties;
DROP FUNCTION IF EXISTS weed_varieties_audit_fn();

DROP TABLE IF EXISTS weed_varieties_history;
DROP TABLE IF EXISTS weed_varieties;
