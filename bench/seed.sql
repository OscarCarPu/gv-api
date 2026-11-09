-- Synthetic data at a given scale. Run on a database already migrated to the current version.
-- Every count comes in as a psql variable; see bench/scenarios.sh.
\set ON_ERROR_STOP on
BEGIN;

TRUNCATE accounts, budgets, calendar_events, categories, concello_marks, habit_logs, habits,
    outages, plan_blocks, projects, recurring_commitment_skips, recurring_commitments,
    task_dependencies, tasks, time_entries, todos, transactions RESTART IDENTITY CASCADE;

SELECT setseed(0.42);

INSERT INTO accounts (name, total, created_at)
SELECT 'Account ' || g, 0, now() - make_interval(days => :days)
FROM generate_series(1, :n_accounts) g;

INSERT INTO categories (name, type, created_at)
SELECT 'Cat ' || g, (CASE WHEN g <= 3 THEN 'income' ELSE 'expense' END)::transaction_type, now()
FROM generate_series(1, :n_cat_roots) g;

INSERT INTO categories (name, parent_id, type, created_at)
SELECT 'Sub ' || g, p.id, p.type, now()
FROM generate_series(1, :n_cat_children) g
CROSS JOIN LATERAL (
    SELECT id, type FROM categories WHERE parent_id IS NULL ORDER BY random() + g * 0 LIMIT 1
) p;

INSERT INTO transactions (type, amount, account_id, to_account_id, category_id, description, occurred_at, created_at)
SELECT t, round((5 + random() * 200)::numeric, 2), a,
       CASE WHEN t = 'transfer' THEN (a % :n_accounts) + 1 END,
       CASE WHEN t <> 'transfer' THEN (
           SELECT id FROM categories c WHERE c.type = x.t ORDER BY random() + x.g * 0 LIMIT 1
       ) END,
       'tx ' || g, ts, ts
FROM (
    SELECT g,
           (CASE WHEN r < 0.85 THEN 'expense' WHEN r < 0.95 THEN 'income' ELSE 'transfer' END)::transaction_type AS t,
           1 + floor(random() * :n_accounts)::int AS a,
           now() - random() * make_interval(days => :days) AS ts
    FROM (SELECT g, random() AS r FROM generate_series(1, :n_tx) g) s
) x;

INSERT INTO budgets (category_id, month, amount, period, created_at)
SELECT c.id, (date_trunc('month', now()) - make_interval(months => m))::date, 100 + (c.id * 7) % 300, 'monthly', now()
FROM generate_series(0, :budget_months - 1) m
CROSS JOIN (SELECT id FROM categories WHERE type = 'expense' ORDER BY id LIMIT :n_budget_cats) c;

INSERT INTO habits (name, frequency, current_streak, longest_streak, target_min, target_max, recording_required, created_at)
SELECT 'Habit ' || g, (ARRAY['daily', 'daily', 'daily', 'weekly', 'monthly'])[1 + g % 5], 0, 0,
       1, CASE WHEN g % 2 = 0 THEN 5 END, g % 3 = 0, now() - make_interval(days => :days)
FROM generate_series(1, :n_habits) g;

INSERT INTO habit_logs (habit_id, log_date, value)
SELECT h.id, d::date, 1 + floor(random() * 4)
FROM habits h
CROSS JOIN generate_series(now() - make_interval(days => :days), now(), interval '1 day') d
WHERE random() < :hl_prob;

INSERT INTO projects (name, priority, started_at, finished_at)
SELECT 'Project ' || g, 1 + g % 5, now() - random() * make_interval(days => :days),
       CASE WHEN random() < 0.2 THEN now() - random() * interval '30 days' END
FROM generate_series(1, :n_projects) g;

UPDATE projects SET parent_id = 1 + floor(random() * (id - 1))::int WHERE id > :n_proj_roots;

INSERT INTO tasks (project_id, name, due_at, started_at, finished_at, task_type, recurrence, priority, estimate_hours)
SELECT 1 + floor(random() * :n_projects)::int, 'Task ' || g,
       CASE WHEN random() < 0.35 THEN (now() + (random() * 120 - 30) * interval '1 day')::date END,
       s,
       CASE WHEN g <= :n_tasks - :n_open THEN least(now(), s + random() * interval '20 days') END,
       tt, CASE WHEN tt = 'recurring' THEN 7 END, 1 + floor(random() * 5)::int,
       CASE WHEN random() < 0.5 THEN round((0.5 + random() * 8)::numeric, 1) END
FROM (
    SELECT g, now() - (1 - g::float / :n_tasks) * make_interval(days => :days) AS s,
           CASE WHEN random() < 0.04 THEN 'recurring' WHEN random() < 0.1 THEN 'continuous' ELSE 'standard' END AS tt
    FROM generate_series(1, :n_tasks) g
) x;

-- Uniform over tasks, pointing at a nearby older task: no cycles possible. Uniform reproduces
-- production's density of dependencies between open tasks (~0.4 per open task).
INSERT INTO task_dependencies (task_id, depends_on)
SELECT t, t - 1 - floor(random() * least(50, t - 1))::int
FROM (SELECT :n_tasks - floor(random() * (:n_tasks - 1))::int AS t FROM generate_series(1, :n_dep)) a
WHERE t > 1
ON CONFLICT DO NOTHING;

INSERT INTO todos (task_id, name, is_done)
SELECT 1 + floor(random() * :n_tasks)::int, 'Todo ' || g, random() < 0.33
FROM generate_series(1, :n_todos) g;

INSERT INTO time_entries (task_id, started_at, finished_at, comment)
SELECT 1 + floor(random() * :n_tasks)::int, s, s + (15 + random() * 75) * interval '1 minute', NULL
FROM (SELECT now() - interval '2 hours' - random() * make_interval(days => :days) AS s FROM generate_series(1, :n_te)) x;

INSERT INTO recurring_commitments (task_id, label, days_of_week, start_time, end_time, active, created_at, updated_at)
SELECT :n_tasks - g, 'Commitment ' || g, ARRAY[1, 3, 5], time '07:00' + g * interval '1 hour',
       time '07:45' + g * interval '1 hour', true, now(), now()
FROM generate_series(0, 2) g;

INSERT INTO plan_blocks (plan_date, started_at, ended_at, task_id, label, created_at, updated_at)
SELECT (s AT TIME ZONE 'Europe/Madrid')::date, s, s + (30 + floor(random() * 4) * 30) * interval '1 minute',
       CASE WHEN random() < 0.6 THEN 1 + floor(random() * :n_tasks)::int END, 'Block ' || g, now(), now()
FROM (
    SELECT g, date_trunc('hour', now() - make_interval(days => :days)
                 + random() * make_interval(days => :days + :future_days)) AS s
    FROM generate_series(1, :n_pb) g
) x;

INSERT INTO calendar_events (name, start_time, end_time, created_at, updated_at)
SELECT 'Event ' || g, s, s + interval '1 hour', now(), now()
FROM (
    SELECT g, date_trunc('minute', now() - make_interval(days => :ev_days_back)
                 + random() * make_interval(days => :ev_days_back + 365)) AS s
    FROM generate_series(1, :n_ev) g
) x;

INSERT INTO concello_marks (name, visited_on, description, created_at)
SELECT 'Concello ' || g, (now() - random() * make_interval(days => :days))::date, '', now()
FROM generate_series(1, :n_marks) g;

-- The 'start' marker, then alternating up/down windows; the last one is the open "up" window.
INSERT INTO outages (ends_at, kind)
SELECT now() - interval '10 seconds' - (:n_out - g) * make_interval(secs => :days * 86400.0 / :n_out),
       (CASE WHEN g = 1 THEN 'start' WHEN (:n_out - g) % 2 = 0 THEN 'up' ELSE 'down' END)::outage_kind
FROM generate_series(1, :n_out) g;

COMMIT;
ANALYZE;
