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
INSERT INTO habits (name, description, frequency, objective, current_streak, longest_streak) VALUES
    ('Exercise', 'Daily physical activity', 'daily', 1, 3, 5),               -- id=1  daily, target: at least 1 session
    ('Reading', 'Read at least 30 minutes', 'daily', 1, 1, 4),              -- id=2  daily, target: at least 1 session
    ('Meditation', 'Morning meditation session', 'weekly', 3, 2, 2),        -- id=3  weekly, target: 3 sessions/week
    ('Water intake', 'Drink 2L of water daily', 'daily', 2, 7, 14),         -- id=4  daily, target: 2L
    ('Sleep', 'Hours of sleep', 'monthly', 200, 1, 3);                       -- id=5  monthly, target: 200h/month

-- =============================================================================
-- HABIT LOGS (last 21 days for meaningful streak data)
-- =============================================================================
INSERT INTO habit_logs (habit_id, log_date, value) VALUES
    -- Exercise (daily, objective=1): streak of 3 (today, yesterday, day before)
    (1, CURRENT_DATE - INTERVAL '20 days', 1),
    (1, CURRENT_DATE - INTERVAL '19 days', 1),
    (1, CURRENT_DATE - INTERVAL '18 days', 1),
    (1, CURRENT_DATE - INTERVAL '17 days', 1),
    (1, CURRENT_DATE - INTERVAL '16 days', 1),
    (1, CURRENT_DATE - INTERVAL '15 days', 0),   -- broke streak (longest=5)
    (1, CURRENT_DATE - INTERVAL '14 days', 1),
    (1, CURRENT_DATE - INTERVAL '13 days', 1),
    (1, CURRENT_DATE - INTERVAL '12 days', 0),
    (1, CURRENT_DATE - INTERVAL '6 days', 1),
    (1, CURRENT_DATE - INTERVAL '5 days', 1),
    (1, CURRENT_DATE - INTERVAL '4 days', 0),   -- broke streak
    (1, CURRENT_DATE - INTERVAL '3 days', 0),
    (1, CURRENT_DATE - INTERVAL '2 days', 1),
    (1, CURRENT_DATE - INTERVAL '1 day', 1),
    (1, CURRENT_DATE, 1),

    -- Reading (daily, objective=1): streak of 1 (today only, missed yesterday)
    (2, CURRENT_DATE - INTERVAL '10 days', 1),
    (2, CURRENT_DATE - INTERVAL '9 days', 1),
    (2, CURRENT_DATE - INTERVAL '8 days', 1),
    (2, CURRENT_DATE - INTERVAL '7 days', 1),   -- longest=4
    (2, CURRENT_DATE - INTERVAL '6 days', 0),
    (2, CURRENT_DATE - INTERVAL '4 days', 1),
    (2, CURRENT_DATE - INTERVAL '2 days', 1),
    (2, CURRENT_DATE - INTERVAL '1 day', 0),
    (2, CURRENT_DATE, 1),

    -- Meditation (weekly, objective=3): streak of 2 full weeks
    -- Week -2 (3 sessions = meets objective)
    (3, CURRENT_DATE - INTERVAL '20 days', 1),
    (3, CURRENT_DATE - INTERVAL '18 days', 1),
    (3, CURRENT_DATE - INTERVAL '16 days', 1),
    -- Week -1 (4 sessions = meets objective)
    (3, CURRENT_DATE - INTERVAL '13 days', 1),
    (3, CURRENT_DATE - INTERVAL '11 days', 1),
    (3, CURRENT_DATE - INTERVAL '9 days', 1),
    (3, CURRENT_DATE - INTERVAL '8 days', 1),
    -- This week (2 sessions so far, not yet meeting objective of 3)
    (3, CURRENT_DATE - INTERVAL '5 days', 1),
    (3, CURRENT_DATE - INTERVAL '3 days', 1),

    -- Water intake (daily, objective=2): perfect streak of 7 days
    (4, CURRENT_DATE - INTERVAL '6 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '5 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '4 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '3 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '2 days', 2.8),
    (4, CURRENT_DATE - INTERVAL '1 day', 2.2),
    (4, CURRENT_DATE, 2.0),

    -- Sleep (monthly, objective=200h): this month accumulating
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
    (5, CURRENT_DATE, 8.0);

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
-- TIME ENTRIES (last 14 days)
-- Uses CURRENT_DATE (midnight) so hour offsets reliably represent time-of-day
-- =============================================================================
INSERT INTO time_entries (task_id, started_at, finished_at, comment) VALUES
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
