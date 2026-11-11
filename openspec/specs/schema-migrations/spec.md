# schema-migrations Specification

## Purpose

Keeps the gv-api database schema current through a self-contained migration history that the service applies at startup, so that nothing depends on the Go release's migration files once the stack migration is done.

## Requirements

### Requirement: Migrations applied at startup
On startup, before serving requests, the service SHALL apply every pending migration in order, all in one transaction. If a migration fails, the service SHALL log the error, leave the schema as it was, and exit non-zero without serving.

#### Scenario: Fresh database
- **WHEN** the service starts against an empty database
- **THEN** the baseline and any later migration are applied and the service starts serving

#### Scenario: Up-to-date database
- **WHEN** the service starts against a database already at the latest migration
- **THEN** no migration runs and startup continues

#### Scenario: Failing migration
- **WHEN** a migration raises an SQL error
- **THEN** the process logs the error and exits non-zero, and no part of the pending migrations is applied

### Requirement: Self-contained baseline
The first migration SHALL create the schema of the Go release at golang-migrate version 36 from a schema snapshot kept with the migrations. Nothing the service, its image or its tooling uses SHALL read the `db/` directory, so `db/` can be deleted after the cutover.

#### Scenario: Same schema as the Go release
- **WHEN** an empty database is migrated by the service and another by the Go release `d32ab42`
- **THEN** their `pg_dump --schema-only` outputs are equal, apart from each tool's bookkeeping table (`alembic_version` and `schema_migrations`)

#### Scenario: Built without db/
- **WHEN** the image is built and the migrations run from a checkout without the `db/` directory
- **THEN** both succeed

### Requirement: No automatic adoption
The service SHALL NOT infer migration state from golang-migrate's `schema_migrations`. A database that has tables but no migration history SHALL make startup fail, without changing any data.

#### Scenario: Populated database without history
- **WHEN** the service starts on a database migrated by the Go release that has not been stamped
- **THEN** the baseline fails because the objects already exist, the transaction is rolled back, and the process exits non-zero

### Requirement: Cutover stamp as a deployment step
The cutover deploy SHALL mark the existing production database as being at the baseline before the new service starts, and SHALL do so only when the database has no migration history yet. Running the deploy again SHALL leave the stamp unchanged.

#### Scenario: Cutover deploy
- **WHEN** the deploy that switches to the Python image runs against the production database
- **THEN** the database is stamped at the baseline without running any schema statement, and the new service starts with no pending migration

#### Scenario: Later deploys
- **WHEN** a later deploy runs on an already stamped database
- **THEN** the stamp step does nothing

### Requirement: Rollback window to the Go release
The service SHALL NOT modify or drop the `schema_migrations` table. As long as no migration newer than the baseline has been applied, the Go release SHALL start on the production database without error.

#### Scenario: Rollback right after cutover
- **WHEN** the Go image is redeployed on the stamped production database, before any new migration
- **THEN** golang-migrate reads version 36 and starts without error

### Requirement: Migrations under the failover
On the home side with the failover enabled, startup migrations SHALL run only when the failover mode is normal. After a successful restore swaps the database in, the service SHALL apply pending migrations to the restored database before leaving the hold.

#### Scenario: Held at startup
- **WHEN** the service starts on home while the failover state is not `NORMAL`
- **THEN** no migration runs, so an empty or stale database is reported as it is

#### Scenario: After a restore
- **WHEN** a merge finishes and the restored database is swapped into place
- **THEN** pending migrations are applied to it before the app leaves the hold
