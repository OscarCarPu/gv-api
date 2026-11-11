# runtime-lifecycle Specification

## Purpose

Describes how the gv-api process starts, which background work it runs and when, which one-shot commands it offers, and how it stops. This holds for the home server and for the AWS backup server.

## Requirements

### Requirement: Configuration from the environment
The process SHALL read its configuration from the environment with the variable names and defaults of `.env.example`. `LIGHTS_*` variables are no longer read. It SHALL exit non-zero, naming the missing ones, when any of `DATABASE_URL`, `PASSWORD`, `SEMIPRIVATE_PASSWORD`, `JWT_SECRET`, `TOTP_SECRET` or `ALLOWED_ORIGINS` is unset. It SHALL also exit non-zero on an invalid value: wrong type, `FAILOVER_SIDE` not `home` or `aws`, unknown `TIMEZONE`, or inconsistent backup retention.

#### Scenario: Missing secrets
- **WHEN** the process starts without `JWT_SECRET` and `TOTP_SECRET`
- **THEN** it exits non-zero and logs `required env vars not set: JWT_SECRET, TOTP_SECRET`

#### Scenario: Invalid or empty value
- **WHEN** `PORT=abc` is set, or `PORT` is set to an empty string
- **THEN** the first exits non-zero naming `PORT`, and the second starts on the default port

#### Scenario: Leftover lights config
- **WHEN** `LIGHTS_DRIVER=bluez` is still set
- **THEN** the process starts normally and ignores it

### Requirement: Listening and shutdown
The server SHALL listen on `PORT` (default 8080). On SIGINT or SIGTERM it SHALL stop the background workers, stop accepting connections, let in-flight requests finish for up to 10 seconds, and exit 0.

#### Scenario: Deploy restart
- **WHEN** docker sends SIGTERM during a request
- **THEN** that request completes, and the process exits 0 within 10 seconds

### Requirement: Scheduled backups
When `BACKUP_INTERVAL_MS` is greater than 0, the process SHALL take a backup at each wall-clock multiple of the interval. Each backup SHALL be a gzipped plain-SQL `pg_dump`, filed and pruned by the hourly and daily retention, and copied to `BACKUP_S3_BUCKET` when that is set. At most one backup SHALL run at a time across processes. A value of 0 SHALL disable the schedule.

#### Scenario: Five-minute schedule
- **WHEN** the interval is 300000 and the process starts at 10:03
- **THEN** the first scheduled backup runs at 10:05

#### Scenario: Concurrent on-demand run
- **WHEN** `POST /backups` or the `backup` command runs while a scheduled backup is in progress
- **THEN** only one dump is taken at a time, with the same outcome the Go service reported

### Requirement: Uptime heartbeat
On the home side, the process SHALL record a heartbeat every 30 seconds, starting right away. A gap longer than 90 seconds SHALL open a new uptime window. On the AWS side, the heartbeat SHALL not run.

#### Scenario: Restart after an outage
- **WHEN** the process starts more than 90 seconds after its last heartbeat
- **THEN** the previous window stays closed at the last heartbeat and a new one opens

### Requirement: Printer background work
On the home side, for each printer with both a PrusaLink host and an RTSP URL, the process SHALL poll the printer status and start a recording when a print starts and stop it when the print ends. On the AWS side, no printer work SHALL run.

#### Scenario: Print starts
- **WHEN** PrusaLink reports a print has started
- **THEN** a recording starts without any client request and shows in `GET …/recordings`

### Requirement: Failover gating of workers
On the home side with the failover enabled, scheduled backups and heartbeats SHALL run only while the failover mode is normal. Before each run they SHALL re-read the AWS failover state: a non-`NORMAL` state holds the app and skips the run, and an unreadable state lets the run proceed.

#### Scenario: Failover in progress
- **WHEN** the SSM state reads `FAILING_BACK` at a scheduled backup time
- **THEN** the backup is skipped, the app enters hold, and a log line says a failover is in progress

### Requirement: Failover startup gate
On the home side with `FAILOVER_TOKEN` set, the process SHALL read the AWS failover state once at startup. `NORMAL` (or no parameter) runs normally. `PENDING_MERGE` starts the restore. Any other state holds the app. An unreadable state holds the app and retries every 30 seconds. Setting the token without `BACKUP_S3_BUCKET` SHALL be a startup error.

#### Scenario: SSM unreachable at boot
- **WHEN** the process starts and SSM cannot be read
- **THEN** the app is held, and the state is read again every 30 seconds until it can be applied

### Requirement: One-shot commands
The image SHALL offer a `backup` command that takes one backup with the running configuration and exits 0 on success or non-zero on failure, within 10 minutes. It SHALL also offer a `prusa-login` command that logs in to Prusa Connect interactively (asking for the emailed code on stdin) and saves the session to `PRUSA_CONNECT_TOKEN_FILE`. Neither SHALL start the HTTP server.

#### Scenario: Pre-deploy backup
- **WHEN** the deploy workflow runs the `backup` command in the running container
- **THEN** one dump is written to `BACKUP_DIR` and the command exits 0

#### Scenario: Prusa login
- **WHEN** an operator runs `prusa-login` and enters the emailed code
- **THEN** the session file is written and the running API uses it on its next Connect request
