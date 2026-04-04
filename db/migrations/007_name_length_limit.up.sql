-- Truncate existing names that exceed 40 characters
UPDATE habits SET name = LEFT(name, 40) WHERE LENGTH(name) > 40;
UPDATE projects SET name = LEFT(name, 40) WHERE LENGTH(name) > 40;
UPDATE tasks SET name = LEFT(name, 40) WHERE LENGTH(name) > 40;
UPDATE todos SET name = LEFT(name, 40) WHERE LENGTH(name) > 40;

-- Enforce 40-character limit on name columns
ALTER TABLE habits ALTER COLUMN name TYPE VARCHAR(40);
ALTER TABLE projects ALTER COLUMN name TYPE VARCHAR(40);
ALTER TABLE tasks ALTER COLUMN name TYPE VARCHAR(40);
ALTER TABLE todos ALTER COLUMN name TYPE VARCHAR(40);
