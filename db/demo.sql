-- Demo data for gv-api
-- Run with: make demo

-- =============================================================================
-- CLEAN SLATE
-- =============================================================================
-- Delete in dependency order (children before parents)
TRUNCATE time_entries, todos, tasks, projects, habit_logs, habits RESTART IDENTITY CASCADE;

-- =============================================================================
-- HABITS
-- =============================================================================
INSERT INTO habits (name, description, frequency, target_min, target_max, recording_required, current_streak, longest_streak) VALUES
    ('Exercise', 'Daily physical activity', 'daily', 1, NULL, true, 3, 5),               -- id=1  daily, target: at least 1 session
    ('Reading', 'Read at least 30 minutes', 'daily', 1, NULL, true, 1, 4),              -- id=2  daily, target: at least 1 session
    ('Meditation', 'Morning meditation session', 'weekly', 3, NULL, true, 2, 2),         -- id=3  weekly, target: 3 sessions/week
    ('Water intake', 'Drink 2L of water daily', 'daily', 2, NULL, true, 7, 14),          -- id=4  daily, target: 2L
    ('Sleep', 'Hours of sleep', 'daily', NULL, NULL, true, 0, 0),                        -- id=5  daily, no target (pure tracking)
    ('Weight', 'Body weight in kg', 'daily', 60, 80, false, 0, 0),                       -- id=6  daily, range target, carry-forward
    ('Running', 'Kilometers run', 'monthly', 50, NULL, true, 2, 2);                      -- id=7  monthly, target: 50km/month

-- =============================================================================
-- HABIT LOGS (last 30 days for meaningful history and streak data)
-- =============================================================================
INSERT INTO habit_logs (habit_id, log_date, value) VALUES
    -- Exercise (daily, target_min=1): streak of 3 (today, yesterday, day before)
    (1, CURRENT_DATE - INTERVAL '29 days', 1),
    (1, CURRENT_DATE - INTERVAL '28 days', 1),
    (1, CURRENT_DATE - INTERVAL '27 days', 1),
    (1, CURRENT_DATE - INTERVAL '26 days', 1),
    (1, CURRENT_DATE - INTERVAL '25 days', 1),
    (1, CURRENT_DATE - INTERVAL '24 days', 0),   -- broke streak (longest=5)
    (1, CURRENT_DATE - INTERVAL '23 days', 1),
    (1, CURRENT_DATE - INTERVAL '22 days', 1),
    (1, CURRENT_DATE - INTERVAL '21 days', 0),
    (1, CURRENT_DATE - INTERVAL '20 days', 1),
    (1, CURRENT_DATE - INTERVAL '19 days', 1),
    (1, CURRENT_DATE - INTERVAL '18 days', 1),
    (1, CURRENT_DATE - INTERVAL '17 days', 0),
    (1, CURRENT_DATE - INTERVAL '16 days', 1),
    (1, CURRENT_DATE - INTERVAL '15 days', 1),
    (1, CURRENT_DATE - INTERVAL '14 days', 0),
    (1, CURRENT_DATE - INTERVAL '13 days', 1),
    (1, CURRENT_DATE - INTERVAL '12 days', 1),
    (1, CURRENT_DATE - INTERVAL '11 days', 0),
    (1, CURRENT_DATE - INTERVAL '10 days', 1),
    (1, CURRENT_DATE - INTERVAL '9 days', 1),
    (1, CURRENT_DATE - INTERVAL '8 days', 1),
    (1, CURRENT_DATE - INTERVAL '7 days', 1),
    (1, CURRENT_DATE - INTERVAL '6 days', 1),
    (1, CURRENT_DATE - INTERVAL '5 days', 1),
    (1, CURRENT_DATE - INTERVAL '4 days', 0),   -- broke streak
    (1, CURRENT_DATE - INTERVAL '3 days', 0),
    (1, CURRENT_DATE - INTERVAL '2 days', 1),
    (1, CURRENT_DATE - INTERVAL '1 day', 1),
    (1, CURRENT_DATE, 1),

    -- Reading (daily, target_min=1): streak of 1 (today only, missed yesterday)
    (2, CURRENT_DATE - INTERVAL '29 days', 1),
    (2, CURRENT_DATE - INTERVAL '28 days', 0),
    (2, CURRENT_DATE - INTERVAL '27 days', 1),
    (2, CURRENT_DATE - INTERVAL '26 days', 1),
    (2, CURRENT_DATE - INTERVAL '25 days', 0),
    (2, CURRENT_DATE - INTERVAL '24 days', 1),
    (2, CURRENT_DATE - INTERVAL '23 days', 1),
    (2, CURRENT_DATE - INTERVAL '22 days', 1),
    (2, CURRENT_DATE - INTERVAL '21 days', 0),
    (2, CURRENT_DATE - INTERVAL '20 days', 1),
    (2, CURRENT_DATE - INTERVAL '19 days', 0),
    (2, CURRENT_DATE - INTERVAL '18 days', 1),
    (2, CURRENT_DATE - INTERVAL '17 days', 1),
    (2, CURRENT_DATE - INTERVAL '16 days', 1),
    (2, CURRENT_DATE - INTERVAL '15 days', 1),   -- longest=4
    (2, CURRENT_DATE - INTERVAL '14 days', 0),
    (2, CURRENT_DATE - INTERVAL '13 days', 1),
    (2, CURRENT_DATE - INTERVAL '12 days', 0),
    (2, CURRENT_DATE - INTERVAL '10 days', 1),
    (2, CURRENT_DATE - INTERVAL '9 days', 1),
    (2, CURRENT_DATE - INTERVAL '8 days', 1),
    (2, CURRENT_DATE - INTERVAL '7 days', 1),
    (2, CURRENT_DATE - INTERVAL '6 days', 0),
    (2, CURRENT_DATE - INTERVAL '4 days', 1),
    (2, CURRENT_DATE - INTERVAL '2 days', 1),
    (2, CURRENT_DATE - INTERVAL '1 day', 0),
    (2, CURRENT_DATE, 1),

    -- Meditation (weekly, target_min=3): 12 weeks of data, streak of 2 full weeks
    -- Week -12 (2 sessions = does NOT meet target)
    (3, CURRENT_DATE - INTERVAL '84 days', 1),
    (3, CURRENT_DATE - INTERVAL '82 days', 1),
    -- Week -11 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '77 days', 1),
    (3, CURRENT_DATE - INTERVAL '75 days', 1),
    (3, CURRENT_DATE - INTERVAL '73 days', 1),
    -- Week -10 (2 sessions = does NOT meet target)
    (3, CURRENT_DATE - INTERVAL '70 days', 1),
    (3, CURRENT_DATE - INTERVAL '68 days', 1),
    -- Week -9 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '63 days', 1),
    (3, CURRENT_DATE - INTERVAL '61 days', 1),
    (3, CURRENT_DATE - INTERVAL '59 days', 1),
    -- Week -8 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '56 days', 1),
    (3, CURRENT_DATE - INTERVAL '54 days', 1),
    (3, CURRENT_DATE - INTERVAL '52 days', 1),
    -- Week -7 (2 sessions = does NOT meet target)
    (3, CURRENT_DATE - INTERVAL '49 days', 1),
    (3, CURRENT_DATE - INTERVAL '47 days', 1),
    -- Week -6 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '42 days', 1),
    (3, CURRENT_DATE - INTERVAL '40 days', 1),
    (3, CURRENT_DATE - INTERVAL '38 days', 1),
    -- Week -5 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '35 days', 1),
    (3, CURRENT_DATE - INTERVAL '33 days', 1),
    (3, CURRENT_DATE - INTERVAL '31 days', 1),
    -- Week -4 (2 sessions = does NOT meet target)
    (3, CURRENT_DATE - INTERVAL '29 days', 1),
    (3, CURRENT_DATE - INTERVAL '26 days', 1),
    -- Week -3 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '23 days', 1),
    (3, CURRENT_DATE - INTERVAL '21 days', 1),
    (3, CURRENT_DATE - INTERVAL '19 days', 1),
    -- Week -2 (3 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '16 days', 1),
    (3, CURRENT_DATE - INTERVAL '14 days', 1),
    (3, CURRENT_DATE - INTERVAL '12 days', 1),
    -- Week -1 (4 sessions = meets target)
    (3, CURRENT_DATE - INTERVAL '10 days', 1),
    (3, CURRENT_DATE - INTERVAL '9 days', 1),
    (3, CURRENT_DATE - INTERVAL '8 days', 1),
    (3, CURRENT_DATE - INTERVAL '7 days', 1),
    -- This week (2 sessions so far, not yet meeting target of 3)
    (3, CURRENT_DATE - INTERVAL '5 days', 1),
    (3, CURRENT_DATE - INTERVAL '3 days', 1),

    -- Water intake (daily, target_min=2): perfect streak of 7 days (gap before that)
    (4, CURRENT_DATE - INTERVAL '29 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '28 days', 1.5),  -- below target
    (4, CURRENT_DATE - INTERVAL '27 days', 2.2),
    (4, CURRENT_DATE - INTERVAL '26 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '25 days', 1.8),  -- below target
    (4, CURRENT_DATE - INTERVAL '24 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '23 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '22 days', 2.3),
    (4, CURRENT_DATE - INTERVAL '21 days', 1.0),  -- below target
    (4, CURRENT_DATE - INTERVAL '20 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '19 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '18 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '17 days', 1.5),  -- below target
    (4, CURRENT_DATE - INTERVAL '16 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '15 days', 2.2),
    (4, CURRENT_DATE - INTERVAL '14 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '13 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '12 days', 1.8),  -- below target
    (4, CURRENT_DATE - INTERVAL '11 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '10 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '9 days', 1.5),   -- below target
    (4, CURRENT_DATE - INTERVAL '8 days', 1.2),   -- below target, broke streak
    (4, CURRENT_DATE - INTERVAL '6 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '5 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '4 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '3 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '2 days', 2.8),
    (4, CURRENT_DATE - INTERVAL '1 day', 2.2),
    (4, CURRENT_DATE, 2.0),

    -- Sleep (daily, no targets): pure tracking, 30 days
    (5, CURRENT_DATE - INTERVAL '29 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '28 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '27 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '26 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '25 days', 6.0),
    (5, CURRENT_DATE - INTERVAL '24 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '23 days', 8.5),
    (5, CURRENT_DATE - INTERVAL '22 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '21 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '20 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '19 days', 6.0),
    (5, CURRENT_DATE - INTERVAL '18 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '17 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '16 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '15 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '14 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '13 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '12 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '11 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '10 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '9 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '8 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '7 days', 6.0),
    (5, CURRENT_DATE - INTERVAL '6 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '5 days', 6.0),
    (5, CURRENT_DATE - INTERVAL '4 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '3 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '2 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '1 day', 7.5),
    (5, CURRENT_DATE, 8.0),

    -- Weight (daily, target_min=60, target_max=80, recording_required=false): gradual downtrend ~73->70kg
    (6, CURRENT_DATE - INTERVAL '29 days', 73.2),
    (6, CURRENT_DATE - INTERVAL '27 days', 73.0),
    (6, CURRENT_DATE - INTERVAL '25 days', 72.8),
    (6, CURRENT_DATE - INTERVAL '23 days', 73.1),
    (6, CURRENT_DATE - INTERVAL '21 days', 72.5),
    (6, CURRENT_DATE - INTERVAL '19 days', 72.3),
    (6, CURRENT_DATE - INTERVAL '17 days', 72.0),
    (6, CURRENT_DATE - INTERVAL '15 days', 71.8),
    (6, CURRENT_DATE - INTERVAL '13 days', 72.5),
    (6, CURRENT_DATE - INTERVAL '12 days', 72.0),
    (6, CURRENT_DATE - INTERVAL '10 days', 71.5),
    (6, CURRENT_DATE - INTERVAL '9 days', 71.8),
    (6, CURRENT_DATE - INTERVAL '7 days', 71.0),
    (6, CURRENT_DATE - INTERVAL '6 days', 70.5),
    (6, CURRENT_DATE - INTERVAL '4 days', 70.8),
    (6, CURRENT_DATE - INTERVAL '3 days', 70.2),
    (6, CURRENT_DATE - INTERVAL '1 day', 69.5),
    (6, CURRENT_DATE, 69.8),

    -- Running (monthly, target_min=50km): 12 months of data, streak of 2 months
    -- 12 months ago (~40km = does NOT meet target)
    (7, CURRENT_DATE - INTERVAL '365 days', 5),
    (7, CURRENT_DATE - INTERVAL '360 days', 8),
    (7, CURRENT_DATE - INTERVAL '355 days', 6),
    (7, CURRENT_DATE - INTERVAL '350 days', 7),
    (7, CURRENT_DATE - INTERVAL '345 days', 4),
    (7, CURRENT_DATE - INTERVAL '340 days', 10),
    -- 11 months ago (~52km = meets target)
    (7, CURRENT_DATE - INTERVAL '335 days', 6),
    (7, CURRENT_DATE - INTERVAL '330 days', 8),
    (7, CURRENT_DATE - INTERVAL '325 days', 5),
    (7, CURRENT_DATE - INTERVAL '320 days', 12),
    (7, CURRENT_DATE - INTERVAL '315 days', 7),
    (7, CURRENT_DATE - INTERVAL '310 days', 10),
    (7, CURRENT_DATE - INTERVAL '305 days', 4),
    -- 10 months ago (~48km = does NOT meet target)
    (7, CURRENT_DATE - INTERVAL '300 days', 8),
    (7, CURRENT_DATE - INTERVAL '295 days', 6),
    (7, CURRENT_DATE - INTERVAL '290 days', 5),
    (7, CURRENT_DATE - INTERVAL '285 days', 10),
    (7, CURRENT_DATE - INTERVAL '280 days', 7),
    (7, CURRENT_DATE - INTERVAL '275 days', 4),
    (7, CURRENT_DATE - INTERVAL '270 days', 8),
    -- 9 months ago (~55km = meets target)
    (7, CURRENT_DATE - INTERVAL '265 days', 6),
    (7, CURRENT_DATE - INTERVAL '260 days', 10),
    (7, CURRENT_DATE - INTERVAL '255 days', 8),
    (7, CURRENT_DATE - INTERVAL '250 days', 12),
    (7, CURRENT_DATE - INTERVAL '245 days', 7),
    (7, CURRENT_DATE - INTERVAL '240 days', 5),
    (7, CURRENT_DATE - INTERVAL '235 days', 7),
    -- 8 months ago (~53km = meets target)
    (7, CURRENT_DATE - INTERVAL '230 days', 8),
    (7, CURRENT_DATE - INTERVAL '225 days', 5),
    (7, CURRENT_DATE - INTERVAL '220 days', 10),
    (7, CURRENT_DATE - INTERVAL '215 days', 6),
    (7, CURRENT_DATE - INTERVAL '210 days', 12),
    (7, CURRENT_DATE - INTERVAL '205 days', 8),
    (7, CURRENT_DATE - INTERVAL '200 days', 4),
    -- 7 months ago (~42km = does NOT meet target)
    (7, CURRENT_DATE - INTERVAL '195 days', 6),
    (7, CURRENT_DATE - INTERVAL '190 days', 8),
    (7, CURRENT_DATE - INTERVAL '185 days', 5),
    (7, CURRENT_DATE - INTERVAL '180 days', 10),
    (7, CURRENT_DATE - INTERVAL '175 days', 7),
    (7, CURRENT_DATE - INTERVAL '170 days', 6),
    -- 6 months ago (~56km = meets target)
    (7, CURRENT_DATE - INTERVAL '165 days', 8),
    (7, CURRENT_DATE - INTERVAL '160 days', 10),
    (7, CURRENT_DATE - INTERVAL '155 days', 6),
    (7, CURRENT_DATE - INTERVAL '150 days', 12),
    (7, CURRENT_DATE - INTERVAL '145 days', 5),
    (7, CURRENT_DATE - INTERVAL '140 days', 8),
    (7, CURRENT_DATE - INTERVAL '135 days', 7),
    -- 5 months ago (~45km = does NOT meet target)
    (7, CURRENT_DATE - INTERVAL '130 days', 5),
    (7, CURRENT_DATE - INTERVAL '125 days', 8),
    (7, CURRENT_DATE - INTERVAL '120 days', 6),
    (7, CURRENT_DATE - INTERVAL '115 days', 10),
    (7, CURRENT_DATE - INTERVAL '110 days', 7),
    (7, CURRENT_DATE - INTERVAL '105 days', 9),
    -- 4 months ago (~51km = meets target)
    (7, CURRENT_DATE - INTERVAL '100 days', 8),
    (7, CURRENT_DATE - INTERVAL '95 days', 5),
    (7, CURRENT_DATE - INTERVAL '90 days', 12),
    (7, CURRENT_DATE - INTERVAL '87 days', 6),
    (7, CURRENT_DATE - INTERVAL '84 days', 10),
    (7, CURRENT_DATE - INTERVAL '81 days', 7),
    (7, CURRENT_DATE - INTERVAL '78 days', 3),
    -- 3 months ago (~55km = meets target)
    (7, CURRENT_DATE - INTERVAL '85 days', 5),
    (7, CURRENT_DATE - INTERVAL '82 days', 3),
    (7, CURRENT_DATE - INTERVAL '79 days', 8),
    (7, CURRENT_DATE - INTERVAL '76 days', 4),
    (7, CURRENT_DATE - INTERVAL '73 days', 6),
    (7, CURRENT_DATE - INTERVAL '70 days', 10),
    (7, CURRENT_DATE - INTERVAL '67 days', 3),
    (7, CURRENT_DATE - INTERVAL '64 days', 7),
    (7, CURRENT_DATE - INTERVAL '61 days', 5),
    (7, CURRENT_DATE - INTERVAL '59 days', 4),
    -- 2 months ago (~62km = meets target)
    (7, CURRENT_DATE - INTERVAL '55 days', 8),
    (7, CURRENT_DATE - INTERVAL '52 days', 5),
    (7, CURRENT_DATE - INTERVAL '49 days', 12),
    (7, CURRENT_DATE - INTERVAL '46 days', 3),
    (7, CURRENT_DATE - INTERVAL '43 days', 6),
    (7, CURRENT_DATE - INTERVAL '40 days', 10),
    (7, CURRENT_DATE - INTERVAL '37 days', 4),
    (7, CURRENT_DATE - INTERVAL '34 days', 7),
    (7, CURRENT_DATE - INTERVAL '31 days', 5),
    (7, CURRENT_DATE - INTERVAL '30 days', 2),
    -- Last month (~62km = meets target)
    (7, CURRENT_DATE - INTERVAL '29 days', 8),
    (7, CURRENT_DATE - INTERVAL '27 days', 5),
    (7, CURRENT_DATE - INTERVAL '24 days', 12),
    (7, CURRENT_DATE - INTERVAL '21 days', 3),
    (7, CURRENT_DATE - INTERVAL '18 days', 6),
    (7, CURRENT_DATE - INTERVAL '15 days', 10),
    (7, CURRENT_DATE - INTERVAL '12 days', 4),
    (7, CURRENT_DATE - INTERVAL '9 days', 7),
    (7, CURRENT_DATE - INTERVAL '6 days', 5),
    (7, CURRENT_DATE - INTERVAL '3 days', 2),
    -- This month (in progress, ~23km so far)
    (7, CURRENT_DATE - INTERVAL '2 days', 8),
    (7, CURRENT_DATE - INTERVAL '1 day', 10),
    (7, CURRENT_DATE, 5);

-- =============================================================================
-- PROJECTS
-- =============================================================================
INSERT INTO projects (name, description, due_at, started_at) VALUES
    ('Website Redesign', 'Redesign the company website with new branding', CURRENT_DATE + INTERVAL '30 days', NOW() - INTERVAL '10 days');  -- id=1

INSERT INTO projects (parent_id, name, description, due_at, started_at) VALUES
    (1, 'Homepage Revamp', 'Redesign the homepage layout and content', CURRENT_DATE + INTERVAL '20 days', NOW() - INTERVAL '8 days');       -- id=2

INSERT INTO projects (parent_id, name, description, started_at) VALUES
    (2, 'Hero Section', 'Design and implement the new hero section', NOW() - INTERVAL '5 days');                                             -- id=3

INSERT INTO projects (name, description, due_at, started_at) VALUES
    ('API v2', 'Build version 2 of the public API', CURRENT_DATE + INTERVAL '60 days', NOW() - INTERVAL '5 days');                           -- id=4

INSERT INTO projects (name, description, due_at) VALUES
    ('Mobile App', 'Cross-platform mobile application', CURRENT_DATE + INTERVAL '90 days');                                                   -- id=5

INSERT INTO projects (parent_id, name, description, due_at) VALUES
    (5, 'Onboarding Flow', 'Design and build the onboarding screens', CURRENT_DATE + INTERVAL '45 days');                                    -- id=6

INSERT INTO projects (name, description, due_at, started_at) VALUES
    ('Data Pipeline', 'Build ETL pipeline for analytics data', CURRENT_DATE + INTERVAL '45 days', NOW() - INTERVAL '14 days');               -- id=7

-- =============================================================================
-- TASKS
-- =============================================================================

-- Website Redesign (project 1)
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (1, 'Create wireframes', 'Design wireframes for all pages', CURRENT_DATE + INTERVAL '5 days', NOW() - INTERVAL '8 days'),                -- id=1
    (1, 'Set up CI/CD', 'Configure deployment pipeline', CURRENT_DATE + INTERVAL '10 days', NOW() - INTERVAL '3 days');                      -- id=2

INSERT INTO tasks (project_id, name, description, started_at) VALUES
    (1, 'Content migration', 'Migrate existing content to new layout', NOW() - INTERVAL '2 days');                                            -- id=3

INSERT INTO tasks (project_id, name, description, started_at, finished_at) VALUES
    (1, 'Choose color palette', 'Select brand colors', NOW() - INTERVAL '9 days', NOW() - INTERVAL '7 days');                                -- id=4

-- Homepage Revamp (project 2)
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (2, 'Design homepage layout', 'Create mockups for the new homepage', CURRENT_DATE + INTERVAL '7 days', NOW() - INTERVAL '6 days');       -- id=5

-- Hero Section (project 3)
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (3, 'Implement hero animation', 'Build the hero section parallax effect', CURRENT_DATE + INTERVAL '12 days', NOW() - INTERVAL '2 days'); -- id=6

-- API v2 (project 4)
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (4, 'Design API schema', 'Define OpenAPI spec for v2', CURRENT_DATE + INTERVAL '14 days', NOW() - INTERVAL '5 days'),                    -- id=7
    (4, 'Auth system', 'Implement OAuth2 authentication', CURRENT_DATE + INTERVAL '21 days', NULL);                                           -- id=8

INSERT INTO tasks (project_id, name, description) VALUES
    (4, 'Rate limiting', 'Implement rate limiting middleware');                                                                                 -- id=9

-- Mobile App (project 5)
INSERT INTO tasks (project_id, name, description, due_at) VALUES
    (5, 'Research frameworks', 'Evaluate React Native vs Flutter', CURRENT_DATE + INTERVAL '7 days');                                         -- id=10

INSERT INTO tasks (project_id, name, description) VALUES
    (5, 'Push notifications', 'Set up push notification service');                                                                             -- id=11

-- Onboarding Flow (project 6)
INSERT INTO tasks (project_id, name, description, due_at) VALUES
    (6, 'Design onboarding screens', 'Mockups for the 5-step onboarding', CURRENT_DATE + INTERVAL '20 days');                                -- id=12

-- Data Pipeline (project 7)
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (7, 'Set up data ingestion', 'Configure Kafka consumers for raw event data', CURRENT_DATE + INTERVAL '10 days', NOW() - INTERVAL '14 days'),   -- id=13
    (7, 'Build transformation layer', 'Data cleaning, normalization, and enrichment', CURRENT_DATE + INTERVAL '20 days', NOW() - INTERVAL '12 days'), -- id=14
    (7, 'Create reporting views', 'SQL views and materialized views for dashboards', CURRENT_DATE + INTERVAL '30 days', NOW() - INTERVAL '10 days'), -- id=15
    (7, 'Write pipeline tests', 'Integration tests for the full ETL pipeline', CURRENT_DATE + INTERVAL '35 days', NOW() - INTERVAL '8 days'),       -- id=16
    (7, 'Deploy pipeline to staging', 'Set up staging env and run validation', CURRENT_DATE + INTERVAL '40 days', NOW() - INTERVAL '6 days');        -- id=17

-- Standalone tasks (no project)
INSERT INTO tasks (name, description, due_at, started_at) VALUES
    ('Fix server logs', 'Investigate and fix log rotation issue', CURRENT_DATE + INTERVAL '2 days', NOW() - INTERVAL '1 day'),               -- id=18
    ('Update dependencies', 'Bump Go modules to latest versions', CURRENT_DATE + INTERVAL '3 days', NULL),                                    -- id=19
    ('Write blog post', 'Draft a blog post about the new API', CURRENT_DATE + INTERVAL '14 days', NULL);                                     -- id=20

INSERT INTO tasks (name, description, started_at) VALUES
    ('Clean up Docker images', 'Remove unused Docker images from registry', NOW() - INTERVAL '1 day');                                        -- id=21

INSERT INTO tasks (name, description) VALUES
    ('Review PR backlog', 'Go through open pull requests and review');                                                                         -- id=22

INSERT INTO tasks (name, description, started_at, finished_at) VALUES
    ('Set up monitoring', 'Configure Prometheus and Grafana', NOW() - INTERVAL '15 days', NOW() - INTERVAL '12 days');                        -- id=23

-- =============================================================================
-- TODOS
-- =============================================================================
INSERT INTO todos (task_id, name, is_done) VALUES
    -- Create wireframes (task 1)
    (1, 'Homepage wireframe', TRUE),
    (1, 'About page wireframe', TRUE),
    (1, 'Contact page wireframe', FALSE),
    -- Set up CI/CD (task 2)
    (2, 'Set up GitHub Actions', TRUE),
    (2, 'Configure staging deploy', FALSE),
    (2, 'Configure production deploy', FALSE),
    -- Design homepage layout (task 5)
    (5, 'Desktop layout', TRUE),
    (5, 'Mobile layout', FALSE),
    -- Design API schema (task 7)
    (7, 'Define endpoints', TRUE),
    (7, 'Write request/response schemas', FALSE),
    (7, 'Add pagination spec', FALSE),
    -- Research frameworks (task 10)
    (10, 'Try React Native', FALSE),
    (10, 'Try Flutter', FALSE),
    (10, 'Write comparison doc', FALSE),
    -- Set up data ingestion (task 13)
    (13, 'Configure Kafka topics', TRUE),
    (13, 'Write consumer group logic', TRUE),
    (13, 'Add dead letter queue', FALSE),
    -- Build transformation layer (task 14)
    (14, 'Null handling rules', TRUE),
    (14, 'Timestamp normalization', FALSE),
    (14, 'Field enrichment from lookup tables', FALSE),
    -- Create reporting views (task 15)
    (15, 'Daily summary view', TRUE),
    (15, 'Weekly rollup view', FALSE),
    -- Write pipeline tests (task 16)
    (16, 'End-to-end happy path test', FALSE),
    (16, 'Error recovery test', FALSE),
    -- Deploy pipeline to staging (task 17)
    (17, 'Terraform staging config', FALSE),
    (17, 'Smoke test script', FALSE),
    -- Fix server logs (task 18)
    (18, 'Check logrotate config', TRUE),
    (18, 'Test with high volume', FALSE);

-- =============================================================================
-- TIME ENTRIES (last 90 days)
-- Uses CURRENT_DATE (midnight) so hour offsets reliably represent time-of-day
-- =============================================================================
INSERT INTO time_entries (task_id, started_at, finished_at, comment) VALUES
    -- Day 90 (10h): Project kickoff and initial planning
    (1,  CURRENT_DATE - INTERVAL '90 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '90 days' + INTERVAL '11 hours', 'Wireframe tool evaluation and setup'),
    (4,  CURRENT_DATE - INTERVAL '90 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '90 days' + INTERVAL '14 hours', 'Brand guidelines review and mood board'),
    (7,  CURRENT_DATE - INTERVAL '90 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '90 days' + INTERVAL '17 hours 30 minutes', 'API architecture brainstorming session'),

    -- Day 89 (12h): Heavy planning day
    (1,  CURRENT_DATE - INTERVAL '89 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '89 days' + INTERVAL '10 hours 30 minutes', 'Competitor wireframe analysis'),
    (7,  CURRENT_DATE - INTERVAL '89 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '89 days' + INTERVAL '13 hours 45 minutes', 'REST vs GraphQL evaluation for API v2'),
    (13, CURRENT_DATE - INTERVAL '89 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '89 days' + INTERVAL '17 hours', 'Data ingestion requirements gathering'),
    (4,  CURRENT_DATE - INTERVAL '89 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '89 days' + INTERVAL '19 hours 30 minutes', 'Typography selection and pairing tests'),

    -- Day 88 (9h): Steady work Saturday
    (1,  CURRENT_DATE - INTERVAL '88 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '88 days' + INTERVAL '12 hours', 'Sketching navigation flow concepts'),
    (13, CURRENT_DATE - INTERVAL '88 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '88 days' + INTERVAL '16 hours', 'Researching Kafka vs RabbitMQ for ingestion'),
    (4,  CURRENT_DATE - INTERVAL '88 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '88 days' + INTERVAL '19 hours 30 minutes', 'Color accessibility compliance check'),

    -- Day 87 (5h): Light Sunday
    (20, CURRENT_DATE - INTERVAL '87 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '87 days' + INTERVAL '12 hours 30 minutes', 'Blog post topic brainstorm and outline'),
    (1,  CURRENT_DATE - INTERVAL '87 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '87 days' + INTERVAL '16 hours 30 minutes', 'Quick wireframe revisions from Friday notes'),

    -- Day 86 (11h): Monday push
    (7,  CURRENT_DATE - INTERVAL '86 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '86 days' + INTERVAL '10 hours 30 minutes', 'API resource naming conventions document'),
    (13, CURRENT_DATE - INTERVAL '86 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '86 days' + INTERVAL '13 hours 45 minutes', 'Kafka cluster sizing and topic design'),
    (5,  CURRENT_DATE - INTERVAL '86 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '86 days' + INTERVAL '17 hours', 'Homepage layout grid system exploration'),
    (2,  CURRENT_DATE - INTERVAL '86 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '86 days' + INTERVAL '18 hours 30 minutes', 'CI pipeline research and tooling evaluation'),

    -- Day 85 (13h): Crunch day
    (13, CURRENT_DATE - INTERVAL '85 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '85 days' + INTERVAL '10 hours', 'Kafka producer prototype with avro schemas'),
    (7,  CURRENT_DATE - INTERVAL '85 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '85 days' + INTERVAL '13 hours', 'Endpoint authentication strategy design'),
    (14, CURRENT_DATE - INTERVAL '85 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '85 days' + INTERVAL '16 hours 30 minutes', 'Transformation layer requirements document'),
    (5,  CURRENT_DATE - INTERVAL '85 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '85 days' + INTERVAL '20 hours', 'Homepage hero section first concepts'),

    -- Day 84 (10h): Solid Wednesday
    (1,  CURRENT_DATE - INTERVAL '84 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '84 days' + INTERVAL '10 hours 30 minutes', 'Dashboard wireframe first pass'),
    (14, CURRENT_DATE - INTERVAL '84 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '84 days' + INTERVAL '13 hours 45 minutes', 'Data mapping specification for transforms'),
    (2,  CURRENT_DATE - INTERVAL '84 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '84 days' + INTERVAL '18 hours', 'GitHub Actions workflow skeleton'),

    -- Day 83 (9h): Thursday
    (13, CURRENT_DATE - INTERVAL '83 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '83 days' + INTERVAL '11 hours', 'Consumer group configuration and testing'),
    (5,  CURRENT_DATE - INTERVAL '83 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '83 days' + INTERVAL '14 hours', 'Homepage responsive breakpoint planning'),
    (7,  CURRENT_DATE - INTERVAL '83 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '83 days' + INTERVAL '17 hours 30 minutes', 'API versioning strategy discussion'),

    -- Day 82 (12h): Friday push
    (14, CURRENT_DATE - INTERVAL '82 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '82 days' + INTERVAL '10 hours', 'Transform pipeline architecture diagram'),
    (1,  CURRENT_DATE - INTERVAL '82 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '82 days' + INTERVAL '12 hours 45 minutes', 'Settings page wireframe'),
    (13, CURRENT_DATE - INTERVAL '82 days' + INTERVAL '13 hours 15 minutes', CURRENT_DATE - INTERVAL '82 days' + INTERVAL '16 hours', 'Schema registry setup and first schemas'),
    (2,  CURRENT_DATE - INTERVAL '82 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '82 days' + INTERVAL '19 hours', 'Build matrix and test runner config'),

    -- Day 81 (6h): Saturday light
    (4,  CURRENT_DATE - INTERVAL '81 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '81 days' + INTERVAL '13 hours', 'Icon set evaluation and selection'),
    (20, CURRENT_DATE - INTERVAL '81 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '81 days' + INTERVAL '17 hours', 'First blog post draft on project vision'),

    -- Day 80: Rest day

    -- Day 79 (11h): Monday back at it
    (7,  CURRENT_DATE - INTERVAL '79 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '79 days' + INTERVAL '10 hours', 'API error handling conventions'),
    (13, CURRENT_DATE - INTERVAL '79 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '79 days' + INTERVAL '13 hours 15 minutes', 'Dead letter queue initial implementation'),
    (14, CURRENT_DATE - INTERVAL '79 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '79 days' + INTERVAL '16 hours 45 minutes', 'Field mapping engine first prototype'),
    (6,  CURRENT_DATE - INTERVAL '79 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '79 days' + INTERVAL '18 hours 30 minutes', 'Hero section animation research'),

    -- Day 78 (10h): Tuesday
    (15, CURRENT_DATE - INTERVAL '78 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '78 days' + INTERVAL '10 hours 30 minutes', 'Reporting views initial requirements'),
    (1,  CURRENT_DATE - INTERVAL '78 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '78 days' + INTERVAL '13 hours', 'User profile wireframe'),
    (13, CURRENT_DATE - INTERVAL '78 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '78 days' + INTERVAL '16 hours 30 minutes', 'Ingestion retry logic with exponential backoff'),
    (2,  CURRENT_DATE - INTERVAL '78 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '78 days' + INTERVAL '18 hours', 'Lint and format CI step'),

    -- Day 77 (14h): Wednesday marathon
    (14, CURRENT_DATE - INTERVAL '77 days' + INTERVAL '6 hours 30 minutes', CURRENT_DATE - INTERVAL '77 days' + INTERVAL '9 hours 30 minutes', 'Type coercion engine implementation'),
    (7,  CURRENT_DATE - INTERVAL '77 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '77 days' + INTERVAL '12 hours 45 minutes', 'Pagination and filtering spec'),
    (14, CURRENT_DATE - INTERVAL '77 days' + INTERVAL '13 hours 15 minutes', CURRENT_DATE - INTERVAL '77 days' + INTERVAL '16 hours 15 minutes', 'Null handling strategy and edge cases'),
    (15, CURRENT_DATE - INTERVAL '77 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '77 days' + INTERVAL '20 hours 30 minutes', 'Daily summary view SQL design'),

    -- Day 76 (9h): Thursday
    (5,  CURRENT_DATE - INTERVAL '76 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '76 days' + INTERVAL '10 hours 30 minutes', 'Homepage mobile layout prototype'),
    (13, CURRENT_DATE - INTERVAL '76 days' + INTERVAL '11 hours', CURRENT_DATE - INTERVAL '76 days' + INTERVAL '14 hours', 'Backpressure mechanism implementation'),
    (16, CURRENT_DATE - INTERVAL '76 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '76 days' + INTERVAL '17 hours', 'Pipeline test framework evaluation'),

    -- Day 75 (11h): Friday
    (15, CURRENT_DATE - INTERVAL '75 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '75 days' + INTERVAL '10 hours', 'Weekly rollup query design'),
    (6,  CURRENT_DATE - INTERVAL '75 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '75 days' + INTERVAL '13 hours', 'Hero section parallax effect prototype'),
    (7,  CURRENT_DATE - INTERVAL '75 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '75 days' + INTERVAL '16 hours', 'API batch operations design'),
    (16, CURRENT_DATE - INTERVAL '75 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '75 days' + INTERVAL '18 hours 30 minutes', 'Test data generator first version'),

    -- Day 74 (4h): Light Saturday
    (20, CURRENT_DATE - INTERVAL '74 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '74 days' + INTERVAL '12 hours', 'Blog post on architecture decisions'),
    (4,  CURRENT_DATE - INTERVAL '74 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '74 days' + INTERVAL '15 hours', 'Design token documentation'),

    -- Day 73: Rest day

    -- Day 72 (10h): Monday
    (14, CURRENT_DATE - INTERVAL '72 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '72 days' + INTERVAL '11 hours', 'Transformation rule engine refactor'),
    (3,  CURRENT_DATE - INTERVAL '72 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '72 days' + INTERVAL '14 hours', 'Content audit spreadsheet and categorization'),
    (16, CURRENT_DATE - INTERVAL '72 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '72 days' + INTERVAL '18 hours', 'Integration test harness setup'),

    -- Day 71 (12h): Tuesday
    (7,  CURRENT_DATE - INTERVAL '71 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '71 days' + INTERVAL '10 hours', 'OAuth2 provider integration research'),
    (13, CURRENT_DATE - INTERVAL '71 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '71 days' + INTERVAL '13 hours', 'Consumer lag monitoring setup'),
    (15, CURRENT_DATE - INTERVAL '71 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '71 days' + INTERVAL '16 hours 30 minutes', 'Monthly aggregation view design'),
    (1,  CURRENT_DATE - INTERVAL '71 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '71 days' + INTERVAL '19 hours', 'Notification center wireframe'),

    -- Day 70 (8h): Wednesday
    (5,  CURRENT_DATE - INTERVAL '70 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '70 days' + INTERVAL '11 hours', 'Homepage footer design and links'),
    (14, CURRENT_DATE - INTERVAL '70 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '70 days' + INTERVAL '14 hours 30 minutes', 'Timestamp normalization first pass'),
    (2,  CURRENT_DATE - INTERVAL '70 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '70 days' + INTERVAL '16 hours 30 minutes', 'Docker build caching optimization'),

    -- Day 69 (11h): Thursday
    (13, CURRENT_DATE - INTERVAL '69 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '69 days' + INTERVAL '10 hours', 'Schema evolution strategy implementation'),
    (7,  CURRENT_DATE - INTERVAL '69 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '69 days' + INTERVAL '13 hours 15 minutes', 'JWT token validation middleware'),
    (16, CURRENT_DATE - INTERVAL '69 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '69 days' + INTERVAL '16 hours', 'Happy path test coverage for ingestion'),
    (15, CURRENT_DATE - INTERVAL '69 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '69 days' + INTERVAL '18 hours 30 minutes', 'Reporting view index strategy'),

    -- Day 68 (10h): Friday
    (6,  CURRENT_DATE - INTERVAL '68 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '68 days' + INTERVAL '10 hours 30 minutes', 'Hero section CTA button animations'),
    (14, CURRENT_DATE - INTERVAL '68 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '68 days' + INTERVAL '13 hours 45 minutes', 'Currency and locale-aware formatting'),
    (3,  CURRENT_DATE - INTERVAL '68 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '68 days' + INTERVAL '16 hours 15 minutes', 'Legacy content format converter'),
    (17, CURRENT_DATE - INTERVAL '68 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '68 days' + INTERVAL '18 hours', 'Terraform project structure and modules'),

    -- Day 67 (7h): Saturday
    (1,  CURRENT_DATE - INTERVAL '67 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '67 days' + INTERVAL '12 hours', 'Search results page wireframe'),
    (20, CURRENT_DATE - INTERVAL '67 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '67 days' + INTERVAL '15 hours', 'Blog post editing and code samples'),
    (4,  CURRENT_DATE - INTERVAL '67 days' + INTERVAL '15 hours 30 minutes', CURRENT_DATE - INTERVAL '67 days' + INTERVAL '17 hours 30 minutes', 'Dark mode color palette variant'),

    -- Day 66: Rest day

    -- Day 65 (13h): Monday sprint
    (13, CURRENT_DATE - INTERVAL '65 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '65 days' + INTERVAL '10 hours', 'Consumer group rebalancing strategy'),
    (7,  CURRENT_DATE - INTERVAL '65 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '65 days' + INTERVAL '13 hours 15 minutes', 'Rate limiting design with token bucket'),
    (14, CURRENT_DATE - INTERVAL '65 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '65 days' + INTERVAL '17 hours', 'Batch transform with parallel processing'),
    (16, CURRENT_DATE - INTERVAL '65 days' + INTERVAL '17 hours 30 minutes', CURRENT_DATE - INTERVAL '65 days' + INTERVAL '20 hours', 'Error scenario test matrix'),

    -- Day 64 (10h): Tuesday
    (15, CURRENT_DATE - INTERVAL '64 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '64 days' + INTERVAL '10 hours 30 minutes', 'Materialized view refresh scheduling'),
    (5,  CURRENT_DATE - INTERVAL '64 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '64 days' + INTERVAL '13 hours', 'Homepage navigation bar redesign'),
    (17, CURRENT_DATE - INTERVAL '64 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '64 days' + INTERVAL '16 hours 30 minutes', 'VPC and subnet Terraform config'),
    (2,  CURRENT_DATE - INTERVAL '64 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '64 days' + INTERVAL '18 hours', 'CI artifact caching improvements'),

    -- Day 63 (9h): Wednesday
    (1,  CURRENT_DATE - INTERVAL '63 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '63 days' + INTERVAL '11 hours', 'Error state wireframes for all pages'),
    (13, CURRENT_DATE - INTERVAL '63 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '63 days' + INTERVAL '14 hours', 'Ingestion metrics and tracing'),
    (6,  CURRENT_DATE - INTERVAL '63 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '63 days' + INTERVAL '17 hours 30 minutes', 'Hero section video background test'),

    -- Day 62 (11h): Thursday
    (14, CURRENT_DATE - INTERVAL '62 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '62 days' + INTERVAL '10 hours 30 minutes', 'Data validation rules engine'),
    (7,  CURRENT_DATE - INTERVAL '62 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '62 days' + INTERVAL '13 hours', 'CORS and security headers config'),
    (16, CURRENT_DATE - INTERVAL '62 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '62 days' + INTERVAL '16 hours 30 minutes', 'Pipeline timeout and retry tests'),
    (3,  CURRENT_DATE - INTERVAL '62 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '62 days' + INTERVAL '18 hours 30 minutes', 'Content migration dry run on test data'),

    -- Day 61 (8h): Friday light
    (15, CURRENT_DATE - INTERVAL '61 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '61 days' + INTERVAL '11 hours', 'Cross-report filtering design'),
    (5,  CURRENT_DATE - INTERVAL '61 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '61 days' + INTERVAL '14 hours', 'Homepage call-to-action section'),
    (17, CURRENT_DATE - INTERVAL '61 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '61 days' + INTERVAL '16 hours 30 minutes', 'RDS Terraform module and params'),

    -- Day 60 (5h): Saturday
    (4,  CURRENT_DATE - INTERVAL '60 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '60 days' + INTERVAL '12 hours 30 minutes', 'Component spacing and sizing tokens'),
    (20, CURRENT_DATE - INTERVAL '60 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '60 days' + INTERVAL '16 hours', 'Blog post on data pipeline patterns'),

    -- Day 59: Rest day

    -- Day 58 (12h): Monday strong start
    (13, CURRENT_DATE - INTERVAL '58 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '58 days' + INTERVAL '10 hours', 'Partitioning strategy optimization'),
    (7,  CURRENT_DATE - INTERVAL '58 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '58 days' + INTERVAL '13 hours 15 minutes', 'API documentation auto-generation setup'),
    (14, CURRENT_DATE - INTERVAL '58 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '58 days' + INTERVAL '17 hours', 'Lookup table caching layer'),
    (2,  CURRENT_DATE - INTERVAL '58 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '58 days' + INTERVAL '19 hours', 'Staging deploy CI pipeline'),

    -- Day 57 (10h): Tuesday
    (1,  CURRENT_DATE - INTERVAL '57 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '57 days' + INTERVAL '10 hours 30 minutes', 'Loading state wireframes'),
    (16, CURRENT_DATE - INTERVAL '57 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '57 days' + INTERVAL '13 hours 45 minutes', 'Concurrency and race condition tests'),
    (15, CURRENT_DATE - INTERVAL '57 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '57 days' + INTERVAL '18 hours', 'Custom date range report view'),

    -- Day 56 (9h): Wednesday
    (6,  CURRENT_DATE - INTERVAL '56 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '56 days' + INTERVAL '11 hours', 'Hero section tablet breakpoint'),
    (13, CURRENT_DATE - INTERVAL '56 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '56 days' + INTERVAL '14 hours', 'Consumer error classification and routing'),
    (3,  CURRENT_DATE - INTERVAL '56 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '56 days' + INTERVAL '17 hours 30 minutes', 'Content format normalization script'),

    -- Day 55 (14h): Thursday crunch
    (14, CURRENT_DATE - INTERVAL '55 days' + INTERVAL '6 hours 30 minutes', CURRENT_DATE - INTERVAL '55 days' + INTERVAL '9 hours 30 minutes', 'Aggregation transform pipeline'),
    (7,  CURRENT_DATE - INTERVAL '55 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '55 days' + INTERVAL '12 hours 45 minutes', 'Webhook delivery system design'),
    (14, CURRENT_DATE - INTERVAL '55 days' + INTERVAL '13 hours 15 minutes', CURRENT_DATE - INTERVAL '55 days' + INTERVAL '16 hours 30 minutes', 'Window function transforms'),
    (16, CURRENT_DATE - INTERVAL '55 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '55 days' + INTERVAL '20 hours 30 minutes', 'Full pipeline regression test suite'),

    -- Day 54 (8h): Friday
    (5,  CURRENT_DATE - INTERVAL '54 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '54 days' + INTERVAL '11 hours 30 minutes', 'Homepage testimonials section layout'),
    (15, CURRENT_DATE - INTERVAL '54 days' + INTERVAL '12 hours', CURRENT_DATE - INTERVAL '54 days' + INTERVAL '14 hours 30 minutes', 'Report export CSV and PDF design'),
    (17, CURRENT_DATE - INTERVAL '54 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '54 days' + INTERVAL '17 hours', 'ECS task definition Terraform'),

    -- Day 53: Rest day

    -- Day 52 (6h): Light Sunday
    (20, CURRENT_DATE - INTERVAL '52 days' + INTERVAL '11 hours', CURRENT_DATE - INTERVAL '52 days' + INTERVAL '13 hours 30 minutes', 'Blog post final review and images'),
    (1,  CURRENT_DATE - INTERVAL '52 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '52 days' + INTERVAL '17 hours 30 minutes', 'Modal dialog wireframe patterns'),

    -- Day 51 (11h): Monday
    (13, CURRENT_DATE - INTERVAL '51 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '51 days' + INTERVAL '10 hours', 'Ingestion idempotency key implementation'),
    (7,  CURRENT_DATE - INTERVAL '51 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '51 days' + INTERVAL '13 hours', 'API request validation middleware'),
    (14, CURRENT_DATE - INTERVAL '51 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '51 days' + INTERVAL '16 hours 30 minutes', 'Streaming transform mode prototype'),
    (2,  CURRENT_DATE - INTERVAL '51 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '51 days' + INTERVAL '18 hours 30 minutes', 'Test coverage reporting in CI'),

    -- Day 50 (10h): Tuesday
    (15, CURRENT_DATE - INTERVAL '50 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '50 days' + INTERVAL '11 hours', 'Drill-down navigation between reports'),
    (6,  CURRENT_DATE - INTERVAL '50 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '50 days' + INTERVAL '13 hours 30 minutes', 'Hero section accessibility audit'),
    (16, CURRENT_DATE - INTERVAL '50 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '50 days' + INTERVAL '18 hours', 'Performance benchmark test suite'),

    -- Day 49 (9h): Wednesday
    (3,  CURRENT_DATE - INTERVAL '49 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '49 days' + INTERVAL '11 hours', 'Image asset migration script'),
    (14, CURRENT_DATE - INTERVAL '49 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '49 days' + INTERVAL '14 hours 30 minutes', 'Transform error recovery and retry'),
    (7,  CURRENT_DATE - INTERVAL '49 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '49 days' + INTERVAL '17 hours 30 minutes', 'API response compression and caching'),

    -- Day 48 (12h): Thursday
    (13, CURRENT_DATE - INTERVAL '48 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '48 days' + INTERVAL '10 hours', 'Consumer health check endpoint'),
    (5,  CURRENT_DATE - INTERVAL '48 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '48 days' + INTERVAL '12 hours 45 minutes', 'Homepage pricing section layout'),
    (17, CURRENT_DATE - INTERVAL '48 days' + INTERVAL '13 hours 15 minutes', CURRENT_DATE - INTERVAL '48 days' + INTERVAL '16 hours 15 minutes', 'Load balancer and target group Terraform'),
    (15, CURRENT_DATE - INTERVAL '48 days' + INTERVAL '16 hours 45 minutes', CURRENT_DATE - INTERVAL '48 days' + INTERVAL '19 hours', 'Report caching strategy design'),

    -- Day 47 (8h): Friday
    (1,  CURRENT_DATE - INTERVAL '47 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '47 days' + INTERVAL '11 hours 30 minutes', 'Empty state wireframes for all views'),
    (16, CURRENT_DATE - INTERVAL '47 days' + INTERVAL '12 hours', CURRENT_DATE - INTERVAL '47 days' + INTERVAL '14 hours 30 minutes', 'Memory leak test for long-running pipeline'),
    (6,  CURRENT_DATE - INTERVAL '47 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '47 days' + INTERVAL '17 hours', 'Hero section micro-interactions'),

    -- Day 46: Rest day

    -- Day 45 (4h): Light Sunday
    (20, CURRENT_DATE - INTERVAL '45 days' + INTERVAL '11 hours', CURRENT_DATE - INTERVAL '45 days' + INTERVAL '13 hours', 'Blog post on testing strategies'),
    (4,  CURRENT_DATE - INTERVAL '45 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '45 days' + INTERVAL '16 hours', 'Design system component inventory'),

    -- Day 44 (10h): Monday
    (14, CURRENT_DATE - INTERVAL '44 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '44 days' + INTERVAL '11 hours', 'Schema migration transform support'),
    (7,  CURRENT_DATE - INTERVAL '44 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '44 days' + INTERVAL '14 hours', 'API search endpoint with full-text'),
    (13, CURRENT_DATE - INTERVAL '44 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '44 days' + INTERVAL '18 hours', 'Ingestion throughput optimization round 1'),

    -- Day 43 (11h): Tuesday
    (15, CURRENT_DATE - INTERVAL '43 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '43 days' + INTERVAL '10 hours', 'Report scheduling and email delivery'),
    (5,  CURRENT_DATE - INTERVAL '43 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '43 days' + INTERVAL '13 hours', 'Homepage feature grid section'),
    (16, CURRENT_DATE - INTERVAL '43 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '43 days' + INTERVAL '16 hours 30 minutes', 'Data integrity verification tests'),
    (3,  CURRENT_DATE - INTERVAL '43 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '43 days' + INTERVAL '18 hours 30 minutes', 'SEO metadata migration planning'),

    -- Day 42 (9h): Wednesday
    (6,  CURRENT_DATE - INTERVAL '42 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '42 days' + INTERVAL '11 hours', 'Hero section image optimization'),
    (14, CURRENT_DATE - INTERVAL '42 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '42 days' + INTERVAL '14 hours 30 minutes', 'Enrichment from external API sources'),
    (2,  CURRENT_DATE - INTERVAL '42 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '42 days' + INTERVAL '17 hours 30 minutes', 'Canary deployment pipeline step'),

    -- Day 41 (13h): Thursday long day
    (13, CURRENT_DATE - INTERVAL '41 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '41 days' + INTERVAL '10 hours', 'Exactly-once delivery guarantee implementation'),
    (7,  CURRENT_DATE - INTERVAL '41 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '41 days' + INTERVAL '13 hours 15 minutes', 'API bulk operations endpoint'),
    (17, CURRENT_DATE - INTERVAL '41 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '41 days' + INTERVAL '17 hours', 'Secrets manager Terraform integration'),
    (16, CURRENT_DATE - INTERVAL '41 days' + INTERVAL '17 hours 30 minutes', CURRENT_DATE - INTERVAL '41 days' + INTERVAL '20 hours', 'Chaos testing framework setup'),

    -- Day 40 (8h): Friday
    (15, CURRENT_DATE - INTERVAL '40 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '40 days' + INTERVAL '11 hours 30 minutes', 'Report permission model design'),
    (1,  CURRENT_DATE - INTERVAL '40 days' + INTERVAL '12 hours', CURRENT_DATE - INTERVAL '40 days' + INTERVAL '14 hours 30 minutes', 'Onboarding flow wireframe first draft'),
    (14, CURRENT_DATE - INTERVAL '40 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '40 days' + INTERVAL '17 hours', 'Transform performance profiling'),

    -- Day 39 (6h): Saturday
    (4,  CURRENT_DATE - INTERVAL '39 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '39 days' + INTERVAL '12 hours 30 minutes', 'Animation timing and easing standards'),
    (5,  CURRENT_DATE - INTERVAL '39 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '39 days' + INTERVAL '16 hours', 'Homepage scroll behavior design'),

    -- Day 38: Rest day

    -- Day 37 (11h): Monday
    (7,  CURRENT_DATE - INTERVAL '37 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '37 days' + INTERVAL '10 hours 30 minutes', 'API WebSocket subscription design'),
    (13, CURRENT_DATE - INTERVAL '37 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '37 days' + INTERVAL '13 hours 45 minutes', 'Multi-topic consumer orchestration'),
    (16, CURRENT_DATE - INTERVAL '37 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '37 days' + INTERVAL '17 hours', 'Snapshot testing for transform outputs'),
    (15, CURRENT_DATE - INTERVAL '37 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '37 days' + INTERVAL '18 hours 30 minutes', 'Report pagination implementation'),

    -- Day 36 (10h): Tuesday
    (14, CURRENT_DATE - INTERVAL '36 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '36 days' + INTERVAL '11 hours', 'Conditional transform branching logic'),
    (3,  CURRENT_DATE - INTERVAL '36 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '36 days' + INTERVAL '13 hours 30 minutes', 'Content versioning during migration'),
    (6,  CURRENT_DATE - INTERVAL '36 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '36 days' + INTERVAL '16 hours 30 minutes', 'Hero section dark mode variant'),
    (17, CURRENT_DATE - INTERVAL '36 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '36 days' + INTERVAL '18 hours', 'CloudWatch alarms Terraform'),

    -- Day 35 (9h): Wednesday
    (5,  CURRENT_DATE - INTERVAL '35 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '35 days' + INTERVAL '11 hours', 'Homepage sticky header behavior'),
    (13, CURRENT_DATE - INTERVAL '35 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '35 days' + INTERVAL '14 hours 30 minutes', 'Consumer offset management improvements'),
    (7,  CURRENT_DATE - INTERVAL '35 days' + INTERVAL '15 hours', CURRENT_DATE - INTERVAL '35 days' + INTERVAL '17 hours 30 minutes', 'API field selection and sparse responses'),

    -- Day 34 (12h): Thursday
    (15, CURRENT_DATE - INTERVAL '34 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '34 days' + INTERVAL '10 hours', 'Comparative period report design'),
    (14, CURRENT_DATE - INTERVAL '34 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '34 days' + INTERVAL '13 hours 15 minutes', 'Transform DAG execution engine'),
    (16, CURRENT_DATE - INTERVAL '34 days' + INTERVAL '13 hours 45 minutes', CURRENT_DATE - INTERVAL '34 days' + INTERVAL '16 hours 45 minutes', 'Transform unit test coverage push'),
    (2,  CURRENT_DATE - INTERVAL '34 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '34 days' + INTERVAL '19 hours', 'Rollback automation in CI'),

    -- Day 33 (8h): Friday
    (1,  CURRENT_DATE - INTERVAL '33 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '33 days' + INTERVAL '11 hours 30 minutes', 'Table component wireframe with sorting'),
    (6,  CURRENT_DATE - INTERVAL '33 days' + INTERVAL '12 hours', CURRENT_DATE - INTERVAL '33 days' + INTERVAL '14 hours', 'Hero section loading skeleton'),
    (17, CURRENT_DATE - INTERVAL '33 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '33 days' + INTERVAL '17 hours', 'Auto-scaling group Terraform config'),

    -- Day 32: Rest day

    -- Day 31 (5h): Light Sunday
    (20, CURRENT_DATE - INTERVAL '31 days' + INTERVAL '11 hours', CURRENT_DATE - INTERVAL '31 days' + INTERVAL '13 hours 30 minutes', 'Blog post on deployment automation'),
    (4,  CURRENT_DATE - INTERVAL '31 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '31 days' + INTERVAL '16 hours 30 minutes', 'Responsive typography scale'),

    -- Day 30 (10h): Monday
    (13, CURRENT_DATE - INTERVAL '30 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '30 days' + INTERVAL '10 hours 30 minutes', 'Dead letter queue retry dashboard'),
    (7,  CURRENT_DATE - INTERVAL '30 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '30 days' + INTERVAL '13 hours 30 minutes', 'API changelog and deprecation policy'),
    (14, CURRENT_DATE - INTERVAL '30 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '30 days' + INTERVAL '18 hours', 'Transform orchestration with dependencies'),

    -- Day 29 (11h): Tuesday
    (16, CURRENT_DATE - INTERVAL '29 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '29 days' + INTERVAL '10 hours', 'End-to-end pipeline acceptance tests'),
    (5,  CURRENT_DATE - INTERVAL '29 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '29 days' + INTERVAL '13 hours', 'Homepage animation performance tuning'),
    (15, CURRENT_DATE - INTERVAL '29 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '29 days' + INTERVAL '16 hours', 'Report template system design'),
    (3,  CURRENT_DATE - INTERVAL '29 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '29 days' + INTERVAL '18 hours 30 minutes', 'Redirect mapping for migrated content'),

    -- Day 28 (9h): Wednesday
    (14, CURRENT_DATE - INTERVAL '28 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '28 days' + INTERVAL '11 hours', 'Transform versioning and rollback'),
    (7,  CURRENT_DATE - INTERVAL '28 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '28 days' + INTERVAL '13 hours 45 minutes', 'API health and readiness endpoints'),
    (6,  CURRENT_DATE - INTERVAL '28 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '28 days' + INTERVAL '17 hours 30 minutes', 'Hero section final polish and handoff'),

    -- Day 27 (13h): Thursday marathon
    (13, CURRENT_DATE - INTERVAL '27 days' + INTERVAL '6 hours 30 minutes', CURRENT_DATE - INTERVAL '27 days' + INTERVAL '9 hours 30 minutes', 'Ingestion rate limiter implementation'),
    (17, CURRENT_DATE - INTERVAL '27 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '27 days' + INTERVAL '12 hours 45 minutes', 'Staging environment validation scripts'),
    (15, CURRENT_DATE - INTERVAL '27 days' + INTERVAL '13 hours 15 minutes', CURRENT_DATE - INTERVAL '27 days' + INTERVAL '16 hours', 'Reporting data warehouse schema'),
    (16, CURRENT_DATE - INTERVAL '27 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '27 days' + INTERVAL '19 hours 30 minutes', 'Load testing with production-like data'),

    -- Day 26 (8h): Friday
    (1,  CURRENT_DATE - INTERVAL '26 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '26 days' + INTERVAL '11 hours', 'Form validation pattern wireframes'),
    (14, CURRENT_DATE - INTERVAL '26 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '26 days' + INTERVAL '14 hours', 'Transform monitoring and alerting'),
    (2,  CURRENT_DATE - INTERVAL '26 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '26 days' + INTERVAL '17 hours', 'Blue-green deployment pipeline'),

    -- Day 25 (7h): Saturday
    (5,  CURRENT_DATE - INTERVAL '25 days' + INTERVAL '9 hours 30 minutes', CURRENT_DATE - INTERVAL '25 days' + INTERVAL '12 hours', 'Homepage SEO meta and structure'),
    (20, CURRENT_DATE - INTERVAL '25 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '25 days' + INTERVAL '15 hours', 'Blog post on monitoring best practices'),
    (4,  CURRENT_DATE - INTERVAL '25 days' + INTERVAL '15 hours 30 minutes', CURRENT_DATE - INTERVAL '25 days' + INTERVAL '17 hours', 'Shadow and elevation design tokens'),

    -- Day 24: Rest day

    -- Day 23 (10h): Monday
    (7,  CURRENT_DATE - INTERVAL '23 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '23 days' + INTERVAL '11 hours', 'API client SDK auto-generation'),
    (13, CURRENT_DATE - INTERVAL '23 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '23 days' + INTERVAL '14 hours', 'Ingestion exactly-once dedup check'),
    (17, CURRENT_DATE - INTERVAL '23 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '23 days' + INTERVAL '18 hours', 'Production Terraform plan review'),

    -- Day 22 (12h): Tuesday
    (14, CURRENT_DATE - INTERVAL '22 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '22 days' + INTERVAL '10 hours', 'Transform config hot-reload support'),
    (16, CURRENT_DATE - INTERVAL '22 days' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '22 days' + INTERVAL '13 hours', 'Pipeline canary test automation'),
    (15, CURRENT_DATE - INTERVAL '22 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '22 days' + INTERVAL '16 hours 30 minutes', 'Report sharing and collaboration features'),
    (3,  CURRENT_DATE - INTERVAL '22 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '22 days' + INTERVAL '19 hours', 'Content migration final validation suite'),

    -- Day 21 (9h): Wednesday
    (1,  CURRENT_DATE - INTERVAL '21 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '21 days' + INTERVAL '11 hours', 'Accessibility audit wireframe updates'),
    (6,  CURRENT_DATE - INTERVAL '21 days' + INTERVAL '11 hours 30 minutes', CURRENT_DATE - INTERVAL '21 days' + INTERVAL '14 hours', 'Hero section performance optimization'),
    (13, CURRENT_DATE - INTERVAL '21 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '21 days' + INTERVAL '17 hours 30 minutes', 'Ingestion throughput optimization round 2'),

    -- Day 20 (11h): Thursday
    (7,  CURRENT_DATE - INTERVAL '20 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '20 days' + INTERVAL '10 hours 30 minutes', 'API integration test suite setup'),
    (14, CURRENT_DATE - INTERVAL '20 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '20 days' + INTERVAL '13 hours 45 minutes', 'Transform multi-step pipeline chaining'),
    (17, CURRENT_DATE - INTERVAL '20 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '20 days' + INTERVAL '16 hours 15 minutes', 'Terraform state management and locking'),
    (16, CURRENT_DATE - INTERVAL '20 days' + INTERVAL '16 hours 30 minutes', CURRENT_DATE - INTERVAL '20 days' + INTERVAL '18 hours 30 minutes', 'Pipeline monitoring dashboard tests'),

    -- Day 19 (8h): Friday
    (5,  CURRENT_DATE - INTERVAL '19 days' + INTERVAL '9 hours', CURRENT_DATE - INTERVAL '19 days' + INTERVAL '11 hours 30 minutes', 'Homepage final responsive polish'),
    (15, CURRENT_DATE - INTERVAL '19 days' + INTERVAL '12 hours', CURRENT_DATE - INTERVAL '19 days' + INTERVAL '14 hours', 'Report data freshness indicator'),
    (2,  CURRENT_DATE - INTERVAL '19 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '19 days' + INTERVAL '17 hours', 'Feature flag integration in CI'),

    -- Day 18 (6h): Saturday
    (10, CURRENT_DATE - INTERVAL '18 days' + INTERVAL '10 hours', CURRENT_DATE - INTERVAL '18 days' + INTERVAL '12 hours 30 minutes', 'React Native navigation patterns'),
    (20, CURRENT_DATE - INTERVAL '18 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '18 days' + INTERVAL '16 hours', 'Blog post on API design lessons'),

    -- Day 17: Rest day

    -- Day 16 (10h): Monday pre-existing sprint
    (23, CURRENT_DATE - INTERVAL '16 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '16 days' + INTERVAL '10 hours 30 minutes', 'Grafana dashboard layout and panels'),
    (13, CURRENT_DATE - INTERVAL '16 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '16 days' + INTERVAL '13 hours 45 minutes', 'Ingestion consumer group tuning'),
    (7,  CURRENT_DATE - INTERVAL '16 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '16 days' + INTERVAL '18 hours', 'API v2 integration with frontend proxy'),

    -- Day 15 (11h): Tuesday
    (14, CURRENT_DATE - INTERVAL '15 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '15 days' + INTERVAL '10 hours 30 minutes', 'Transform pipeline graceful shutdown'),
    (23, CURRENT_DATE - INTERVAL '15 days' + INTERVAL '10 hours 45 minutes', CURRENT_DATE - INTERVAL '15 days' + INTERVAL '13 hours', 'Grafana alerting rules config'),
    (16, CURRENT_DATE - INTERVAL '15 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '15 days' + INTERVAL '16 hours 30 minutes', 'Pipeline SLA compliance test'),
    (4,  CURRENT_DATE - INTERVAL '15 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '15 days' + INTERVAL '18 hours 30 minutes', 'Final color palette approval prep'),

    -- Day 14 (10h): Color palette + wireframes + data ingestion
    (13, CURRENT_DATE - INTERVAL '14 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '14 days' + INTERVAL '9 hours 30 minutes', 'Kafka topic config and consumer skeleton'),
    (4,  CURRENT_DATE - INTERVAL '14 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '14 days' + INTERVAL '11 hours', 'Color palette research and mood board'),
    (4,  CURRENT_DATE - INTERVAL '14 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '14 days' + INTERVAL '12 hours', 'Narrowed down to 3 palette options'),
    (1,  CURRENT_DATE - INTERVAL '14 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '14 days' + INTERVAL '15 hours 30 minutes', 'Initial wireframe sketches for homepage'),
    (13, CURRENT_DATE - INTERVAL '14 days' + INTERVAL '16 hours', CURRENT_DATE - INTERVAL '14 days' + INTERVAL '19 hours', 'Consumer group logic and offset management'),

    -- Day 13 (13h): Wireframes + API planning + ingestion deep dive
    (13, CURRENT_DATE - INTERVAL '13 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '13 days' + INTERVAL '9 hours 30 minutes', 'Dead letter queue design and implementation'),
    (1,  CURRENT_DATE - INTERVAL '13 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '13 days' + INTERVAL '12 hours 45 minutes', 'Refined homepage wireframe with feedback'),
    (7,  CURRENT_DATE - INTERVAL '13 days' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '13 days' + INTERVAL '16 hours 30 minutes', 'API v2 brainstorming and resource mapping'),
    (13, CURRENT_DATE - INTERVAL '13 days' + INTERVAL '17 hours', CURRENT_DATE - INTERVAL '13 days' + INTERVAL '20 hours', 'Ingestion error handling, retries, and backpressure'),

    -- Day 12 (12h): Wireframes + monitoring + transformation layer marathon
    (14, CURRENT_DATE - INTERVAL '12 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '12 days' + INTERVAL '9 hours', 'Transformation layer architecture and data flow design'),
    (1,  CURRENT_DATE - INTERVAL '12 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '12 days' + INTERVAL '11 hours', 'About page wireframe'),
    (23, CURRENT_DATE - INTERVAL '12 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '12 days' + INTERVAL '13 hours 15 minutes', 'Final Grafana dashboard tweaks'),
    (14, CURRENT_DATE - INTERVAL '12 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '12 days' + INTERVAL '17 hours', 'Null handling and type coercion rules'),
    (14, CURRENT_DATE - INTERVAL '12 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '12 days' + INTERVAL '19 hours', 'Timestamp normalization across all sources'),

    -- Day 11 (9h): Homepage layout + API + reporting views
    (15, CURRENT_DATE - INTERVAL '11 days' + INTERVAL '8 hours', CURRENT_DATE - INTERVAL '11 days' + INTERVAL '9 hours 30 minutes', 'Reporting views requirements gathering'),
    (5,  CURRENT_DATE - INTERVAL '11 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '11 days' + INTERVAL '12 hours', 'Homepage layout exploration'),
    (7,  CURRENT_DATE - INTERVAL '11 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '11 days' + INTERVAL '17 hours', 'Initial API schema design session'),
    (15, CURRENT_DATE - INTERVAL '11 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '11 days' + INTERVAL '19 hours', 'Daily summary view first draft'),

    -- Day 10 (14h): API design + reporting views crunch
    (15, CURRENT_DATE - INTERVAL '10 days' + INTERVAL '6 hours 30 minutes', CURRENT_DATE - INTERVAL '10 days' + INTERVAL '9 hours', 'Weekly rollup view design and prototyping'),
    (7,  CURRENT_DATE - INTERVAL '10 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '10 days' + INTERVAL '12 hours', 'Endpoint definitions and request schemas'),
    (15, CURRENT_DATE - INTERVAL '10 days' + INTERVAL '12 hours 15 minutes', CURRENT_DATE - INTERVAL '10 days' + INTERVAL '15 hours', 'Materialized view refresh strategy'),
    (7,  CURRENT_DATE - INTERVAL '10 days' + INTERVAL '15 hours 15 minutes', CURRENT_DATE - INTERVAL '10 days' + INTERVAL '17 hours', 'Response schemas and error formats'),
    (16, CURRENT_DATE - INTERVAL '10 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '10 days' + INTERVAL '20 hours 30 minutes', 'Pipeline test framework setup and first tests'),

    -- Day 9 (11h): Homepage + content migration + pipeline tests
    (16, CURRENT_DATE - INTERVAL '9 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '9 days' + INTERVAL '9 hours', 'Test fixtures and sample data generation'),
    (5,  CURRENT_DATE - INTERVAL '9 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '9 days' + INTERVAL '12 hours', 'Desktop layout mockup refinement'),
    (3,  CURRENT_DATE - INTERVAL '9 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '9 days' + INTERVAL '15 hours', 'Content audit and migration planning'),
    (16, CURRENT_DATE - INTERVAL '9 days' + INTERVAL '15 hours 15 minutes', CURRENT_DATE - INTERVAL '9 days' + INTERVAL '17 hours 30 minutes', 'End-to-end happy path test skeleton'),
    (15, CURRENT_DATE - INTERVAL '9 days' + INTERVAL '17 hours 45 minutes', CURRENT_DATE - INTERVAL '9 days' + INTERVAL '19 hours 30 minutes', 'Index tuning for reporting views'),

    -- Day 8 (8h): Mixed work + pipeline tests
    (16, CURRENT_DATE - INTERVAL '8 days' + INTERVAL '8 hours 30 minutes', CURRENT_DATE - INTERVAL '8 days' + INTERVAL '9 hours 30 minutes', 'Error recovery test scenarios'),
    (1,  CURRENT_DATE - INTERVAL '8 days' + INTERVAL '9 hours 45 minutes', CURRENT_DATE - INTERVAL '8 days' + INTERVAL '10 hours 45 minutes', 'Contact page wireframe draft'),
    (3,  CURRENT_DATE - INTERVAL '8 days' + INTERVAL '11 hours', CURRENT_DATE - INTERVAL '8 days' + INTERVAL '13 hours', 'Content migration scripting'),
    (7,  CURRENT_DATE - INTERVAL '8 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '8 days' + INTERVAL '16 hours', 'Pagination spec for API v2'),
    (17, CURRENT_DATE - INTERVAL '8 days' + INTERVAL '16 hours 15 minutes', CURRENT_DATE - INTERVAL '8 days' + INTERVAL '18 hours 15 minutes', 'Terraform staging config started'),

    -- Day 7 (13.5h): Hero section + CI/CD + staging deploy marathon
    (17, CURRENT_DATE - INTERVAL '7 days' + INTERVAL '6 hours 30 minutes', CURRENT_DATE - INTERVAL '7 days' + INTERVAL '9 hours', 'Staging environment networking and IAM setup'),
    (6,  CURRENT_DATE - INTERVAL '7 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '7 days' + INTERVAL '12 hours', 'Hero animation prototype with GSAP'),
    (2,  CURRENT_DATE - INTERVAL '7 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '7 days' + INTERVAL '15 hours', 'GitHub Actions initial setup and workflow config'),
    (17, CURRENT_DATE - INTERVAL '7 days' + INTERVAL '15 hours 15 minutes', CURRENT_DATE - INTERVAL '7 days' + INTERVAL '18 hours', 'Terraform modules for Kafka and DB'),
    (13, CURRENT_DATE - INTERVAL '7 days' + INTERVAL '18 hours 15 minutes', CURRENT_DATE - INTERVAL '7 days' + INTERVAL '20 hours', 'Ingestion throughput benchmarking'),

    -- Day 6 (10.5h): CI/CD + blog post + staging
    (17, CURRENT_DATE - INTERVAL '6 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '6 days' + INTERVAL '9 hours', 'Staging secrets and env vars config'),
    (2,  CURRENT_DATE - INTERVAL '6 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '6 days' + INTERVAL '11 hours', 'Staging deployment config'),
    (20, CURRENT_DATE - INTERVAL '6 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '6 days' + INTERVAL '13 hours', 'Blog post outline and intro draft'),
    (6,  CURRENT_DATE - INTERVAL '6 days' + INTERVAL '14 hours', CURRENT_DATE - INTERVAL '6 days' + INTERVAL '16 hours 30 minutes', 'Hero parallax scroll effect and polish'),
    (17, CURRENT_DATE - INTERVAL '6 days' + INTERVAL '16 hours 45 minutes', CURRENT_DATE - INTERVAL '6 days' + INTERVAL '19 hours', 'First staging deploy attempt and debugging'),

    -- Day 5 (12h): API + server logs + transformation deep work
    (14, CURRENT_DATE - INTERVAL '5 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '5 days' + INTERVAL '9 hours', 'Field enrichment from lookup tables'),
    (7,  CURRENT_DATE - INTERVAL '5 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '5 days' + INTERVAL '12 hours', 'Auth endpoint design for OAuth2 flow'),
    (18, CURRENT_DATE - INTERVAL '5 days' + INTERVAL '13 hours', CURRENT_DATE - INTERVAL '5 days' + INTERVAL '14 hours', 'Investigating log rotation issue'),
    (14, CURRENT_DATE - INTERVAL '5 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '5 days' + INTERVAL '17 hours 30 minutes', 'Enrichment pipeline integration testing'),
    (16, CURRENT_DATE - INTERVAL '5 days' + INTERVAL '17 hours 45 minutes', CURRENT_DATE - INTERVAL '5 days' + INTERVAL '19 hours', 'Pipeline error injection tests'),

    -- Day 4 (15h): Content migration + hero + reporting crunch day
    (15, CURRENT_DATE - INTERVAL '4 days' + INTERVAL '6 hours', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '8 hours 30 minutes', 'Reporting view performance benchmarks'),
    (3,  CURRENT_DATE - INTERVAL '4 days' + INTERVAL '8 hours 45 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '11 hours', 'Testing content migration on staging'),
    (15, CURRENT_DATE - INTERVAL '4 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '14 hours', 'Dashboard query optimization'),
    (6,  CURRENT_DATE - INTERVAL '4 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '16 hours', 'Hero section responsive tweaks'),
    (18, CURRENT_DATE - INTERVAL '4 days' + INTERVAL '16 hours 15 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '17 hours', 'Checked logrotate config'),
    (14, CURRENT_DATE - INTERVAL '4 days' + INTERVAL '17 hours 15 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '19 hours 30 minutes', 'Batch transformation performance tuning'),
    (16, CURRENT_DATE - INTERVAL '4 days' + INTERVAL '19 hours 45 minutes', CURRENT_DATE - INTERVAL '4 days' + INTERVAL '21 hours', 'Pipeline stress test after tuning'),

    -- Day 3 (11h): CI/CD + API + pipeline deploy
    (16, CURRENT_DATE - INTERVAL '3 days' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '3 days' + INTERVAL '9 hours', 'Full regression test suite run'),
    (2,  CURRENT_DATE - INTERVAL '3 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '3 days' + INTERVAL '11 hours', 'Production deployment pipeline'),
    (7,  CURRENT_DATE - INTERVAL '3 days' + INTERVAL '11 hours 15 minutes', CURRENT_DATE - INTERVAL '3 days' + INTERVAL '14 hours', 'Rate limiting design and implementation start'),
    (10, CURRENT_DATE - INTERVAL '3 days' + INTERVAL '14 hours 30 minutes', CURRENT_DATE - INTERVAL '3 days' + INTERVAL '16 hours 30 minutes', 'React Native evaluation and demo app'),
    (17, CURRENT_DATE - INTERVAL '3 days' + INTERVAL '16 hours 45 minutes', CURRENT_DATE - INTERVAL '3 days' + INTERVAL '18 hours 30 minutes', 'Staging validation and smoke tests'),

    -- Day 2 (12.5h): Framework research + Docker + pipeline
    (13, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '7 hours', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '9 hours', 'Ingestion monitoring and alerting setup'),
    (10, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '12 hours', 'Flutter evaluation and comparison notes'),
    (14, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '12 hours 15 minutes', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '14 hours', 'Transformation layer code review fixes'),
    (21, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '14 hours 15 minutes', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '16 hours', 'Docker image audit and tagging'),
    (20, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '16 hours 15 minutes', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '17 hours 15 minutes', 'Blog post API examples section'),
    (17, CURRENT_DATE - INTERVAL '2 days' + INTERVAL '17 hours 30 minutes', CURRENT_DATE - INTERVAL '2 days' + INTERVAL '19 hours 30 minutes', 'Staging deploy monitoring dashboards'),

    -- Day 1 (9.5h yesterday): Log fix + onboarding + pipeline
    (16, CURRENT_DATE - INTERVAL '1 day' + INTERVAL '7 hours 30 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '9 hours', 'Pipeline test coverage analysis'),
    (18, CURRENT_DATE - INTERVAL '1 day' + INTERVAL '9 hours 15 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '10 hours', 'Applied log rotation fix'),
    (12, CURRENT_DATE - INTERVAL '1 day' + INTERVAL '10 hours 15 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '12 hours 30 minutes', 'Onboarding screen mockups - first 3 screens'),
    (3,  CURRENT_DATE - INTERVAL '1 day' + INTERVAL '13 hours 30 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '15 hours 30 minutes', 'Final content migration batch'),
    (7,  CURRENT_DATE - INTERVAL '1 day' + INTERVAL '15 hours 45 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '17 hours 30 minutes', 'API v2 auth flow implementation start'),
    (15, CURRENT_DATE - INTERVAL '1 day' + INTERVAL '17 hours 45 minutes', CURRENT_DATE - INTERVAL '1 day' + INTERVAL '19 hours', 'Reporting view access control review'),

    -- Today (10.5h)
    (13, CURRENT_DATE + INTERVAL '7 hours', CURRENT_DATE + INTERVAL '9 hours', 'Ingestion consumer group rebalancing fix'),
    (17, CURRENT_DATE + INTERVAL '9 hours 15 minutes', CURRENT_DATE + INTERVAL '12 hours', 'Production deploy checklist and runbook'),
    (14, CURRENT_DATE + INTERVAL '12 hours 15 minutes', CURRENT_DATE + INTERVAL '13 hours 45 minutes', 'Transformation layer edge case fixes'),
    (12, CURRENT_DATE + INTERVAL '14 hours', CURRENT_DATE + INTERVAL '15 hours 30 minutes', 'Onboarding screens 4 and 5'),
    (16, CURRENT_DATE + INTERVAL '15 hours 45 minutes', CURRENT_DATE + INTERVAL '16 hours 45 minutes', 'Pipeline load test with production-scale data');
