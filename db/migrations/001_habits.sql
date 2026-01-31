CREATE TABLE habits (
    id INTEGER PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    name TEXT NOT NULL,
    description TEXT
);

CREATE TABLE habit_logs (
    habit_id INTEGER NOT NULL,
    log_date DATE NOT NULL DEFAULT CURRENT_DATE,
    value REAL NOT NULL,
    CONSTRAINT fk_habit FOREIGN KEY (habit_id) REFERENCES habits(id) ON DELETE CASCADE,
    UNIQUE (habit_id, log_date)
);
