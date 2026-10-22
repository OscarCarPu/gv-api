# Trying the failover restore locally

`make up` (and `make reset`) also start `gv_aws`, a [moto](https://github.com/getmoto/moto) container that stands in for the two AWS services home talks to: SSM (the failover state and the dump keys) and S3 (the dumps). `docker-compose.dev-aws.yaml` points gv-api at it and sets `FAILOVER_TOKEN=dev-token`, so the real failover code runs: the start gate, `/failover`, the restore, the swap, and the `/report` API. Nothing here touches real AWS.

The file is for local runs only. The deploy workflow calls `docker compose` directly and never adds it. `make up DEV_AWS=0` leaves it out.

## Rehearsal

```bash
make up                  # gv-api, Postgres and the fake AWS
cd ../gv-web && make up-dev   # the web app on :5173, talking to gv-api on :8080
```

1. **Stage a divergence.** `make failover-scenario` dumps your dev database as the base, makes a copy that plays the backup server (new rows, which take the same ids as the home ones, and an edit to the first task), then makes the home-side changes in your dev database (new rows, and a different edit to the same task). Both dumps and the SSM parameters go to the fake AWS. It asks before touching the database; `YES=1` skips the question.
2. **Play the Lambda.** `make failover-merge` pushes `hold` and then `merge` to gv-api, with the state at `FAILING_BACK` and the dump key in SSM, as the real Lambda would.
3. **Use the web app.** While the restore runs, any page redirects to **Report** and the app is locked. When it ends the app opens with the backup server's data, and the **Report** link lists what home had besides (the new rows and the edit of the first task) until you dismiss it.
4. **Look.** `make failover-status` shows the fake SSM state and what gv-api reports. The old database is kept as `<database>_pre_restore`.
5. **Record the failover.** `make failover-outage` plays the Lambda's last step: it sets the state to `HOME_READY` and posts the two instants. The `outages` table then has the `up` row closed at home's last heartbeat, a `down` row and a `failover` row.
6. **Reset.** `make failover-finish` puts the state back to NORMAL and clears the dump keys. `make reset` wipes the database to start over.

Android: the phone app shows a blocking "Restoring your data" screen while the hold lasts, and leaves it by itself.

## What it covers and what it does not

Covered: gv-api against real SSM and S3 API calls (through moto), `psql` loading the dumps, the report, the database swap, the lock and the report screen.

Not covered: the Lambda itself (its tests are in `aws-failover`), Cloudflare, EC2's `boot.sh`, and the real IAM policies.

## Changing the scenario

`scripts/failover-dev.sh` holds the SQL for both sides. Add a statement to the heredoc of the side you want to change.

Useful overrides: `AWS_ENDPOINT` (default `http://localhost:5000`), `API` (default `http://localhost:8080`), `FAILOVER_TOKEN`, `BUCKET`, and `DBX` (how to reach the database container, default `docker compose exec -T db`).
