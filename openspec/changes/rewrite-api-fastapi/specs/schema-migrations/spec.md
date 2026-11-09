# Spec Delta

## Purpose

Keeps the gv-api database schema current. The service itself applies an ordered migration history at startup, and adopts databases migrated by earlier releases without re-running their history.

## ADDED Requirements

### Requirement: Migrations applied at startup
On startup, before serving requests, the service SHALL apply every pending migration in order. If a migration fails, the service SHALL log the error and exit non-zero without serving.

#### Scenario: Fresh database
- **WHEN** the service starts against an empty database
- **THEN** the full history (the original 001–036 schema, then any later migration) is applied and the service starts serving

#### Scenario: Up-to-date database
- **WHEN** the service starts against a database already at the latest migration
- **THEN** no migration runs and startup continues

#### Scenario: Failing migration
- **WHEN** a migration raises an SQL error
- **THEN** the process logs the error and exits non-zero

### Requirement: Adoption of a database migrated by the Go release
When the database has no record of the new migration history but its golang-migrate `schema_migrations` row is version 36 and not dirty, the service SHALL mark the baseline (the 001–036 schema) as applied without executing it, and then apply any later migration. A dirty or different golang-migrate version SHALL stop startup with a non-zero exit.

#### Scenario: First deploy over the production database
- **WHEN** the new service first starts on a database whose `schema_migrations` is `(36, false)`
- **THEN** no schema statement runs, the baseline is recorded as applied, and a log line reports the adoption

#### Scenario: Unexpected golang-migrate state
- **WHEN** `schema_migrations` holds a dirty row, or a version other than 36
- **THEN** the service logs the found state and exits non-zero without changing the schema

### Requirement: Rollback window to the Go release
The service SHALL NOT modify or drop the `schema_migrations` table. As long as no migration newer than the baseline has been applied, the Go release SHALL start on the same database without error.

#### Scenario: Rollback right after cutover
- **WHEN** the Go image is redeployed on a database the new service adopted, before any new migration
- **THEN** golang-migrate reads version 36 and starts without error

### Requirement: Existing migrations are immutable
The SQL of migrations `001` to `036` SHALL be reused byte for byte as the baseline. Schema changes after the rewrite SHALL be added as new migrations in the single linear history.

#### Scenario: Checksum of existing files
- **WHEN** the rewrite is complete
- **THEN** every file in `db/migrations/` that existed at `d32ab42` is unchanged

### Requirement: Migrations under the failover
On the home side with the failover enabled, startup migrations SHALL run only when the failover mode is normal. After a successful restore swaps the database in, the service SHALL apply pending migrations to the restored database before leaving the hold.

#### Scenario: Held at startup
- **WHEN** the service starts on home while the failover state is not `NORMAL`
- **THEN** no migration runs, so an empty or stale database is reported as it is

#### Scenario: After a restore
- **WHEN** a merge finishes and the restored database is swapped into place
- **THEN** pending migrations are applied to it before the app leaves the hold
