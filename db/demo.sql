-- Demo data for gv-api
-- Run with: make demo

-- Habits
INSERT INTO habits (name, description) VALUES
    ('Exercise', 'Daily physical activity'),
    ('Reading', 'Read at least 30 minutes'),
    ('Meditation', 'Morning meditation session'),
    ('Water intake', 'Drink 2L of water daily'),
    ('Sleep', 'Hours of sleep')
ON CONFLICT DO NOTHING;

-- Habit logs (last 7 days)
INSERT INTO habit_logs (habit_id, log_date, value) VALUES
    (1, CURRENT_DATE - INTERVAL '6 days', 1),
    (1, CURRENT_DATE - INTERVAL '5 days', 1),
    (1, CURRENT_DATE - INTERVAL '4 days', 0),
    (1, CURRENT_DATE - INTERVAL '3 days', 1),
    (1, CURRENT_DATE - INTERVAL '2 days', 1),
    (1, CURRENT_DATE - INTERVAL '1 day', 0),
    (1, CURRENT_DATE, 1),
    (2, CURRENT_DATE - INTERVAL '6 days', 1),
    (2, CURRENT_DATE - INTERVAL '4 days', 1),
    (2, CURRENT_DATE - INTERVAL '2 days', 1),
    (2, CURRENT_DATE, 1),
    (3, CURRENT_DATE - INTERVAL '5 days', 1),
    (3, CURRENT_DATE - INTERVAL '3 days', 1),
    (3, CURRENT_DATE - INTERVAL '1 day', 1),
    (4, CURRENT_DATE - INTERVAL '6 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '5 days', 1.5),
    (4, CURRENT_DATE - INTERVAL '4 days', 2.5),
    (4, CURRENT_DATE - INTERVAL '3 days', 2.0),
    (4, CURRENT_DATE - INTERVAL '2 days', 1.8),
    (4, CURRENT_DATE - INTERVAL '1 day', 2.2),
    (4, CURRENT_DATE, 2.0),
    (5, CURRENT_DATE - INTERVAL '6 days', 7.5),
    (5, CURRENT_DATE - INTERVAL '5 days', 6.0),
    (5, CURRENT_DATE - INTERVAL '4 days', 8.0),
    (5, CURRENT_DATE - INTERVAL '3 days', 7.0),
    (5, CURRENT_DATE - INTERVAL '2 days', 6.5),
    (5, CURRENT_DATE - INTERVAL '1 day', 7.5),
    (5, CURRENT_DATE, 8.0)
ON CONFLICT DO NOTHING;

-- Projects (6 total)
-- id=1: root project
INSERT INTO projects (name, description, due_at, started_at) VALUES
    ('Website Redesign', 'Redesign the company website with new branding', CURRENT_DATE + INTERVAL '30 days', NOW() - INTERVAL '10 days');

-- id=2: child of Website Redesign (1)
INSERT INTO projects (parent_id, name, description, due_at, started_at) VALUES
    (1, 'Homepage Revamp', 'Redesign the homepage layout and content', CURRENT_DATE + INTERVAL '20 days', NOW() - INTERVAL '8 days');

-- id=3: child of Homepage Revamp (2) — grandchild of 1
INSERT INTO projects (parent_id, name, description, started_at) VALUES
    (2, 'Hero Section', 'Design and implement the new hero section', NOW() - INTERVAL '5 days');

-- id=4: another root project
INSERT INTO projects (name, description, due_at, started_at) VALUES
    ('API v2', 'Build version 2 of the public API', CURRENT_DATE + INTERVAL '60 days', NOW() - INTERVAL '5 days');

-- id=5: root project with a child
INSERT INTO projects (name, description, due_at) VALUES
    ('Mobile App', 'Cross-platform mobile application', CURRENT_DATE + INTERVAL '90 days');

-- id=6: child of Mobile App (5)
INSERT INTO projects (parent_id, name, description, due_at) VALUES
    (5, 'Onboarding Flow', 'Design and build the onboarding screens', CURRENT_DATE + INTERVAL '45 days');

-- Tasks on projects
INSERT INTO tasks (project_id, name, description, due_at, started_at) VALUES
    (1, 'Create wireframes', 'Design wireframes for all pages', CURRENT_DATE + INTERVAL '5 days', NOW() - INTERVAL '8 days'),
    (1, 'Set up CI/CD', 'Configure deployment pipeline', CURRENT_DATE + INTERVAL '10 days', NOW() - INTERVAL '3 days'),
    (2, 'Design homepage layout', 'Create mockups for the new homepage', CURRENT_DATE + INTERVAL '7 days', NOW() - INTERVAL '6 days'),
    (3, 'Implement hero animation', 'Build the hero section parallax effect', CURRENT_DATE + INTERVAL '12 days', NOW() - INTERVAL '2 days'),
    (4, 'Design API schema', 'Define OpenAPI spec for v2', CURRENT_DATE + INTERVAL '14 days', NOW() - INTERVAL '5 days'),
    (4, 'Auth system', 'Implement OAuth2 authentication', CURRENT_DATE + INTERVAL '21 days', NULL),
    (5, 'Research frameworks', 'Evaluate React Native vs Flutter', CURRENT_DATE + INTERVAL '7 days', NULL),
    (6, 'Design onboarding screens', 'Mockups for the 5-step onboarding', CURRENT_DATE + INTERVAL '20 days', NULL);

-- Tasks on projects without due date
INSERT INTO tasks (project_id, name, description, started_at) VALUES
    (1, 'Content migration', 'Migrate existing content to new layout', NOW() - INTERVAL '2 days'),
    (4, 'Rate limiting', 'Implement rate limiting middleware', NULL),
    (5, 'Push notifications', 'Set up push notification service', NULL);

-- A finished task on a project
INSERT INTO tasks (project_id, name, description, started_at, finished_at) VALUES
    (1, 'Choose color palette', 'Select brand colors', NOW() - INTERVAL '9 days', NOW() - INTERVAL '7 days');

-- Tasks without project (root tasks)
INSERT INTO tasks (name, description, due_at, started_at) VALUES
    ('Fix server logs', 'Investigate and fix log rotation issue', CURRENT_DATE + INTERVAL '2 days', NOW() - INTERVAL '1 day'),
    ('Update dependencies', 'Bump Go modules to latest versions', CURRENT_DATE + INTERVAL '3 days', NULL),
    ('Write blog post', 'Draft a blog post about the new API', CURRENT_DATE + INTERVAL '14 days', NULL);

-- Root tasks without due date
INSERT INTO tasks (name, description, started_at) VALUES
    ('Clean up Docker images', 'Remove unused Docker images from registry', NOW() - INTERVAL '1 day'),
    ('Review PR backlog', 'Go through open pull requests and review', NULL);

-- A finished root task
INSERT INTO tasks (name, description, started_at, finished_at) VALUES
    ('Set up monitoring', 'Configure Prometheus and Grafana', NOW() - INTERVAL '15 days', NOW() - INTERVAL '12 days');

-- Todos
INSERT INTO todos (task_id, name, is_done) VALUES
    (1, 'Homepage wireframe', TRUE),
    (1, 'About page wireframe', TRUE),
    (1, 'Contact page wireframe', FALSE),
    (2, 'Set up GitHub Actions', TRUE),
    (2, 'Configure staging deploy', FALSE),
    (2, 'Configure production deploy', FALSE),
    (3, 'Desktop layout', TRUE),
    (3, 'Mobile layout', FALSE),
    (5, 'Define endpoints', TRUE),
    (5, 'Write request/response schemas', FALSE),
    (5, 'Add pagination spec', FALSE),
    (7, 'Try React Native', FALSE),
    (7, 'Try Flutter', FALSE),
    (7, 'Write comparison doc', FALSE),
    (10, 'Check logrotate config', TRUE),
    (10, 'Test with high volume', FALSE);

-- Time entries
INSERT INTO time_entries (task_id, started_at, finished_at, comment) VALUES
    (1, NOW() - INTERVAL '8 days', NOW() - INTERVAL '8 days' + INTERVAL '2 hours', 'Initial wireframe sketches'),
    (1, NOW() - INTERVAL '7 days', NOW() - INTERVAL '7 days' + INTERVAL '3 hours', 'Refined homepage wireframe'),
    (1, NOW() - INTERVAL '6 days', NOW() - INTERVAL '6 days' + INTERVAL '1 hour 30 minutes', 'About page wireframe'),
    (2, NOW() - INTERVAL '3 days', NOW() - INTERVAL '3 days' + INTERVAL '1 hour', 'GitHub Actions setup'),
    (3, NOW() - INTERVAL '6 days', NOW() - INTERVAL '6 days' + INTERVAL '2 hours', 'Homepage layout exploration'),
    (4, NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' + INTERVAL '1 hour 30 minutes', 'Hero animation prototype'),
    (5, NOW() - INTERVAL '5 days', NOW() - INTERVAL '5 days' + INTERVAL '4 hours', 'Initial API design session'),
    (5, NOW() - INTERVAL '4 days', NOW() - INTERVAL '4 days' + INTERVAL '2 hours', 'Endpoint definitions'),
    (9, NOW() - INTERVAL '9 days', NOW() - INTERVAL '9 days' + INTERVAL '30 minutes', 'Color palette research'),
    (10, NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day' + INTERVAL '45 minutes', 'Investigating log rotation');

-- Previously active time entry, now finished
INSERT INTO time_entries (task_id, started_at, finished_at, comment) VALUES
    (10, NOW() - INTERVAL '20 minutes', NOW(), 'Fixing log config');
