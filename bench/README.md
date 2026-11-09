# bench

Latency and throughput measurement for gv-api at production size and at projected growth. It talks only to the HTTP contract, so the same run works against the Go release and the Python rewrite.

| File | What |
|---|---|
| `seed.sql` | Synthetic data at a given scale (psql variables), calibrated against production |
| `scenarios.sh` | The scales: `prod` (2026-11-09), `5y` (60 months at production's rates), `5y_x3` (three times those rates) |
| `load/` | Load client: 39 reads and 6 write cycles. Go, standard library only, with its own module |
| `run.sh` | Seeds one database per scenario, starts the API against it, measures, and stores the results |
| `results/<label>/` | Per scenario: `*.jsonl` (one line per endpoint), `*.txt`, `*.tables.txt` (row counts), `*.process.json` (startup, RSS, CPU), `*.api.log` |

## Running

Needs the `gv_db` container running (`docker compose up -d db`) and Go installed for the client. Everything happens in `gv_bench_*` databases. The development database is not touched.

```bash
# Go release (the reference baseline is go-d32ab42)
go build -o /tmp/gv-api-go ./cmd/api
bench/run.sh /tmp/gv-api-go go-<sha> prod 5y 5y_x3

# Python rewrite
bench/run.sh "gv-api serve" py-<sha> prod 5y 5y_x3
```

The API command runs in the foreground from the repository root, with its configuration in the environment (background work turned off).

The first run builds `gv_bench_template` from `db/migrations/*.up.sql`, with `schema_migrations` at the latest version. `5y_x3` takes several minutes, mostly in `by-due-date` and `slack`.

## Notes

- **Noise:** single endpoints vary up to ±50% between runs. Compare the median of 3 runs.
- **Hardware:** the homelab (i5-8500T) is about 1.6× slower per core than the i7-1360P the baseline was measured on.
- Results and findings are in `openspec/changes/rewrite-api-fastapi/benchmarks.md`.
