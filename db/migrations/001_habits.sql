-- 1. Create custom types for your enums
CREATE TYPE habit_value_type AS ENUM ('boolean', 'numeric');
CREATE TYPE habit_frequency AS ENUM ('daily', 'weekly', 'monthly');
CREATE TYPE habit_comparison_type AS ENUM ('equals', 'greater_than', 'less_than', 'greater_equal_than', 'less_equal_than', 'in_range');

-- 2. Create the habits table
CREATE TABLE habits (
    id INTEGER PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    name TEXT NOT NULL,
    description TEXT,
    value_type habit_value_type NOT NULL,
    unit TEXT,
    frequency habit_frequency NOT NULL,
    target_value REAL,
    target_min REAL,
    target_max REAL,
    comparison_type habit_comparison_type,
    start_date DATE DEFAULT CURRENT_DATE,
    end_date DATE,
    default_value REAL,
    streak_strict BOOLEAN DEFAULT FALSE,
    icon TEXT NOT NULL,
    big_step REAL,
    small_step REAL
);

-- 3. Create the logs table
CREATE TABLE habit_logs (
    habit_id INTEGER NOT NULL,
    log_date DATE NOT NULL DEFAULT CURRENT_DATE,
    value REAL NOT NULL,
    CONSTRAINT fk_habit FOREIGN KEY (habit_id) REFERENCES habits(id) ON DELETE CASCADE,
    UNIQUE (habit_id, log_date)
);
