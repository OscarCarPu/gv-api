# bench

Latency measurement for gv-api at production size and at projected growth, over HTTP.

| File | What |
|---|---|
| `seed.sql` | Synthetic data at a given scale (psql variables), calibrated against production |
| `scenarios.sh` | The scales: `prod` (2026-11-09), `5y` (60 months at production's rates), `5y_x3` (three times those rates) |
| `load.py` | Load client: sequential reads and write cycles (create, then delete), one JSON line per endpoint |
| `run.sh` | Seeds one database per scenario, starts the API against it, measures, and stores the results |
| `results/<label>/` | Per scenario: `*.jsonl` (one line per endpoint), `*.txt`, `*.tables.txt` (row counts), `*.process.json` (startup, RSS, CPU), `*.api.log` |

## Running

Needs the `gv_db` container running (`docker compose up -d db`). Everything happens in `gv_bench_*` databases; the development database is not touched.

```bash
bench/run.sh <label> prod 5y 5y_x3
ONLY=finance.overview,finance.budgets bench/run.sh <label> 5y   # just those endpoints
```

The API runs from the repository root with its configuration in the environment and background work turned off. The first run builds `gv_bench_template` with `alembic upgrade head`. `5y_x3` takes several minutes, mostly in `by-due-date` and `slack`.

## Notes

- **Noise:** single endpoints vary up to ±50% between runs, more on a busy machine. Compare the median of 3 runs.
- **Hardware:** the homelab (i5-8500T) is about 1.6× slower per core than an i7-1360P laptop.
- `results/` is not versioned.
