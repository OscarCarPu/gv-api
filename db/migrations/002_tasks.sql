CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    parent_id INTEGER REFERENCES projects(id),
    name TEXT NOT NULL,
    description TEXT,
    due_at DATE,
    started_at TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE TABLE tasks (
    id SERIAL PRIMARY KEY,
    project_id INTEGER REFERENCES projects(id),
    name TEXT NOT NULL,
    description TEXT,
    due_at DATE,
    started_at TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE TABLE todos (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    is_done BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE time_entries (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP,
    comment TEXT
);

-- Foreign key indexes (not auto-created by Postgres)
CREATE INDEX idx_projects_parent_id ON projects(parent_id);
CREATE INDEX idx_tasks_project_id ON tasks(project_id);
CREATE INDEX idx_todos_task_id ON todos(task_id);
CREATE INDEX idx_time_entries_task_id ON time_entries(task_id);

-- Useful query indexes
CREATE INDEX idx_tasks_unfinished ON tasks(finished_at) WHERE finished_at IS NULL;
CREATE INDEX idx_tasks_due_at ON tasks(due_at) WHERE due_at IS NOT NULL;
CREATE INDEX idx_time_entries_started_at ON time_entries(started_at);
