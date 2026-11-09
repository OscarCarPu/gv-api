# Spec Delta

## Purpose

Sends every error the service logs to a Telegram chat, so the operator learns about failed backups, restores, migrations, integrations and request errors without watching the logs.

## ADDED Requirements

### Requirement: Telegram delivery of error logs
When `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are both set, every log record at ERROR level or above SHALL be sent to that chat. Each message SHALL include the failover side, the log message, the error text, and the `request_id` when there is one.

#### Scenario: Failed scheduled backup
- **WHEN** a scheduled backup fails and logs `backup: scheduled backup failed`
- **THEN** the chat receives a message with that text, the error and the side (`home` or `aws`)

#### Scenario: Failed request
- **WHEN** a request fails with a 500 and its error is logged
- **THEN** the chat message carries that request's `request_id`

### Requirement: Alerts disabled without configuration
When either Telegram variable is unset, the service SHALL send no alerts, SHALL log once at startup that alerts are off, and SHALL otherwise behave the same.

#### Scenario: No bot configured
- **WHEN** the service starts without `TELEGRAM_BOT_TOKEN`
- **THEN** it starts normally and logs at info level that Telegram alerts are disabled

### Requirement: Batching and deduplication
Error records SHALL be batched and sent at most once every 10 seconds. Records with the same message within one batch SHALL be collapsed into one line with a repeat count. Each Telegram message SHALL stay within Telegram's 4096-character limit, and a truncated message SHALL say so.

#### Scenario: Error storm
- **WHEN** the database goes down and 200 requests log the same error within 10 seconds
- **THEN** the chat receives one message showing that error with a count of 200

### Requirement: Alerts never affect the service
Sending alerts SHALL never block or fail a request or a background job. If Telegram cannot be reached, the batch SHALL be dropped and the failure logged at WARNING level, which is not itself alerted. The bot token SHALL never appear in logs or alerts.

#### Scenario: Telegram unreachable
- **WHEN** the Telegram API times out
- **THEN** requests keep their normal latency, and a WARNING line reports the dropped batch without the token

### Requirement: Fatal startup errors are alerted
When the process is about to exit non-zero because of an error at startup (such as a failed migration or an unreachable failover dependency), it SHALL try to send the pending alerts first, waiting no more than 5 seconds.

#### Scenario: Migration failure on deploy
- **WHEN** a migration fails during startup
- **THEN** the chat receives the error before the process exits
