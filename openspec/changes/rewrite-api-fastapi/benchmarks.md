# Performance baseline: Go release `d32ab42`

Measured on 2026-11-09, before the FastAPI rewrite. This is the reference for the latency gate (design D15 and task 18.2). The raw data is in `bench/results/go-d32ab42/`, and `bench/README.md` explains how to reproduce it.

## Summary

- **Today the API is fast.** At production size every read answers in under 30 ms at p50 (p95 ≤ 44 ms). The slowest are `tasks.by-due-date` and `slack`, at about 30 ms.
- **At 5 years (expected growth) it is still comfortable.** The worst p95 (median of 3 runs) are 90–95 ms (`by-due-date`, `slack`, `networth.5y`), which is roughly 150 ms on the homelab.
- **At 5 years ×3 (headroom), four endpoints become slow:** `by-due-date` and `slack` (median p95 237 ms, with a peak run at 376), `networth` (238–292 ms) and `estimation` (130 ms). On the homelab that is 0.2–0.5 s.
- **Almost all the time is spent in Postgres, not in Go.** In the heavy endpoints Go uses 0.5–12 ms of CPU per request against 25–200 ms in Postgres. The only endpoint bound by the application is `finance.transactions.all`, which serialises the whole history without pagination: 1.1 MB at 5 years and 3.2 MB at ×3.
- **Implication for Python: low risk.** Running the same SQL, latency will be close to Go's. The JSON encoding planned in D7 was measured on the real payload: it produces output **identical** to Go's and costs about the same (41 ms against 34 ms of Go CPU for 5,000 transactions).

## Method

- **What was measured:** the Go binary built from `d32ab42`, running against Postgres 15 (the `gv_db` container) with a separate database per scenario. Background work was off: backups, printers and failover. The heartbeat stayed on.
- **Load:** `bench/load` (Go, standard library only).
  - 39 reads and 6 writes.
  - Reads: 10 warm-up requests, 100 sequential requests (p50, p95, p99), then 400 more at concurrency 8 (req/s).
  - Writes: 100 create cycles, deleting the created row each time, with the delete measured separately.
- **Data:** synthetic, from `bench/seed.sql`, calibrated against production (read-only counts taken on 2026-11-09, about 75 days of use):
  - **prod:** production's row counts.
  - **5y:** prod plus 60 months at the monthly rates observed in production (about 80 transactions, 215 time entries, 27 habit logs, 50 tasks, 48 todos and 75 plan blocks per month).
  - **5y×3:** the same with three times those rates.
  - **Check:** dependencies between open tasks are 43 in the prod seed against 40 in real production. A first version of the seed concentrated them on recent tasks, three times more than reality, and pushed `by-due-date` to about 900 ms. It was corrected to a uniform distribution.
- **Hardware:** an i7-1360P. The homelab (i5-8500T) is **about 1.6× slower** per core (calibrated with the same Python micro-benchmark on both machines). The "homelab" column multiplies by 1.6. It is an estimate, not a measurement on the server.

### Scenario sizes

| Table | prod | 5y | 5y×3 |
|---|---:|---:|---:|
| transactions | 173 | 5,000 | 14,600 |
| time_entries | 501 | 13,400 | 39,200 |
| habit_logs | 60 | 1,602 | 5,033 |
| tasks | 188 | 3,200 | 9,200 |
| todos | 121 | 3,000 | 8,800 |
| task_dependencies | 71 | 1,194 | 3,492 |
| plan_blocks | 616 | 5,100 | 14,000 |
| calendar_events | 662 | 1,900 | 4,300 |
| projects | 30 | 150 | 400 |
| budgets | 9 | 496 | 1,240 |
| outages | 5 | 365 | 1,081 |
| concello_marks | 8 | 188 | 313 |
| habits | 5 | 10 | 15 |
| categories | 30 | 45 | 75 |
| accounts | 10 | 15 | 20 |
| **database size** | 9.3 MB | 14.1 MB | 23.2 MB |

## Results (ms)

| Endpoint | prod p50 / p95 | 5y p50 / p95 | 5y×3 p50 / p95 | 5y on homelab p95 (est.) | Body 5y | req/s 5y (c=8) |
|---|---:|---:|---:|---:|---:|---:|
| `health` | 0.3 / 0.4 | 0.3 / 0.4 | 0.3 / 0.4 | 1 | – | 12084 |
| `habits.daily` | 8.8 / 12.4 | 60.2 / 68.8 | 106.5 / 117.8 | 110 | 1.9 KB | 51 |
| `habits.history.1y` | 1.2 / 1.5 | 1.7 / 2.0 | 1.8 / 2.3 | 3 | 1.1 KB | 3899 |
| `tasks.tree` | 7.0 / 9.0 | 5.2 / 8.6 | 14.3 / 19.5 | 14 | 48 KB | 847 |
| `tasks.projects.roots` | 0.3 / 0.6 | 0.9 / 1.3 | 1.0 / 1.4 | 2 | 3.8 KB | 5526 |
| `tasks.projects.list-fast` | 0.3 / 0.5 | 0.8 / 1.2 | 1.4 / 2.3 | 2 | 5.3 KB | 5837 |
| `tasks.tasks.list-fast` | 0.9 / 1.2 | 2.0 / 3.2 | 4.6 / 5.5 | 5 | 15 KB | 3188 |
| `tasks.project` | 0.9 / 1.1 | 5.3 / 6.4 | 7.6 / 10.6 | 10 | 0.2 KB | 1602 |
| `tasks.project.children` | 5.3 / 7.7 | 30.1 / 45.3 | 35.0 / 48.7 | 73 | 6.4 KB | 113 |
| `tasks.project.parent-candidates` | 0.9 / 1.4 | 2.4 / 2.9 | 4.4 / 6.7 | 5 | 8.4 KB | 2907 |
| `tasks.task` | 1.0 / 1.5 | 1.0 / 1.8 | 1.4 / 1.8 | 3 | 0.3 KB | 4106 |
| `tasks.task.time-entries` | 0.8 / 1.4 | 1.2 / 1.8 | 1.2 / 1.5 | 3 | 1.1 KB | 4739 |
| `tasks.by-due-date` | 29.9 / 43.8 | 90.1 / 101.1 | 277.8 / 376.0 | 162 | 49 KB | 34 |
| `tasks.by-due-date.slack` | 29.2 / 40.4 | 88.5 / 100.2 | 226.4 / 243.7 | 160 | 0.0 KB | 36 |
| `tasks.time-entries.week` | 0.8 / 1.0 | 5.6 / 6.5 | 10.2 / 12.5 | 10 | 18 KB | 1585 |
| `tasks.time-entries.active` | 0.3 / 0.5 | 0.5 / 0.7 | 0.5 / 0.6 | 1 | 0.0 KB | 10453 |
| `tasks.time-entries.summary` | 0.4 / 0.5 | 1.9 / 2.2 | 2.8 / 4.1 | 3 | 0.3 KB | 3543 |
| `tasks.time-entries.history.30d` | 1.8 / 2.4 | 7.9 / 10.4 | 15.8 / 22.5 | 17 | 1.2 KB | 978 |
| `tasks.time-entries.history.5y` | 3.0 / 4.0 | 36.0 / 43.6 | 94.6 / 100.8 | 70 | 2.6 KB | 89 |
| `plan.today` | 1.0 / 1.3 | 2.6 / 3.1 | 5.5 / 6.2 | 5 | 2.1 KB | 2944 |
| `plan.range.2w` | 3.9 / 5.4 | 4.2 / 6.1 | 8.5 / 10.4 | 10 | 16 KB | 1564 |
| `plan.commitments` | 0.3 / 0.5 | 0.5 / 0.6 | 0.5 / 0.6 | 1 | 0.4 KB | 9995 |
| `capacity.30d` | 15.9 / 19.1 | 38.5 / 49.7 | 79.9 / 92.4 | 80 | 2.5 KB | 111 |
| `calendar.month` | 0.4 / 0.6 | 0.5 / 0.8 | 0.7 / 1.0 | 1 | 1.2 KB | 7552 |
| `finance.accounts` | 0.4 / 0.6 | 0.5 / 0.6 | 0.5 / 0.8 | 1 | 1.4 KB | 9694 |
| `finance.categories` | 0.5 / 0.8 | 0.7 / 0.8 | 0.7 / 1.0 | 1 | 4.6 KB | 7083 |
| `finance.overview` | 2.2 / 3.4 | 4.8 / 5.6 | 9.2 / 11.1 | 9 | 16 KB | 1716 |
| `finance.transactions.all` | 1.4 / 3.3 | 28.8 / 35.3 | 64.3 / 86.2 | 56 | 1074 KB | 140 |
| `finance.transactions.month` | 1.0 / 1.9 | 2.4 / 3.2 | 5.0 / 6.0 | 5 | 19 KB | 2809 |
| `finance.budgets` | 1.8 / 2.9 | 7.2 / 8.0 | 9.7 / 17.5 | 13 | 5.4 KB | 1293 |
| `finance.stats.monthly.1y` | 0.5 / 0.6 | 2.0 / 2.3 | 5.4 / 7.3 | 4 | 1.0 KB | 3284 |
| `finance.stats.monthly.5y` | 0.5 / 0.8 | 4.9 / 6.3 | 12.2 / 16.2 | 10 | 4.9 KB | 1516 |
| `finance.stats.by-category.1y` | 1.1 / 1.3 | 2.7 / 3.0 | 6.0 / 7.3 | 5 | 3.7 KB | 2945 |
| `finance.stats.networth.1y` | 7.2 / 8.6 | 63.3 / 71.8 | 235.8 / 265.7 | 115 | 2.3 KB | 51 |
| `finance.stats.networth.5y` | 6.4 / 10.2 | 91.2 / 100.2 | 282.1 / 298.2 | 160 | 2.7 KB | 33 |
| `finance.stats.estimation` | 2.0 / 3.1 | 27.8 / 35.2 | 122.9 / 133.2 | 56 | 0.9 KB | 149 |
| `rutas.marks` | 0.3 / 0.4 | 0.9 / 1.4 | 1.2 / 1.7 | 2 | 16 KB | 3469 |
| `uptime` | 0.7 / 0.9 | 1.5 / 1.9 | 2.6 / 3.3 | 3 | 0.9 KB | 3444 |
| `uptime.windows.30d` | 0.5 / 0.7 | 1.0 / 1.3 | 1.6 / 1.8 | 2 | 1.0 KB | 7989 |
| `w.habits.log` | 15.1 / 17.8 | 27.0 / 32.9 | 31.9 / 35.8 | 53 | 0.0 KB | – |
| `w.finance.transaction` | 6.6 / 8.3 | 7.4 / 8.8 | 7.8 / 9.1 | 14 | 0.2 KB | – |
| `w.tasks.task` | 8.2 / 10.1 | 12.9 / 15.3 | 14.3 / 16.7 | 24 | 0.2 KB | – |
| `w.tasks.time-entry` | 6.0 / 7.5 | 6.2 / 8.0 | 6.0 / 7.9 | 13 | 0.1 KB | – |
| `w.plan.block` | 6.9 / 8.9 | 8.4 / 9.8 | 8.3 / 10.3 | 16 | 0.3 KB | – |
| `w.calendar.event` | 5.8 / 7.5 | 6.7 / 7.8 | 5.8 / 7.4 | 12 | 0.1 KB | – |

`tasks.time-entries.active` answers 404 because there is no active entry. That is the expected behaviour.

## p95 median of 3 runs (gate reference)

The tables above are from the first run. This is the median of `go-d32ab42`, `-r2` and `-r3`, saved in `bench/results/go-d32ab42-median.json`, which is what the latency gate compares against.

| Endpoint | prod | 5y | 5y×3 |
|---|---:|---:|---:|
| `health` | 0.4 | 0.4 | 0.4 |
| `habits.daily` | 6.6 | 66.0 | 108.4 |
| `habits.history.1y` | 1.0 | 1.8 | 1.8 |
| `tasks.tree` | 8.9 | 8.1 | 19.5 |
| `tasks.projects.roots` | 0.4 | 0.8 | 1.4 |
| `tasks.projects.list-fast` | 0.5 | 1.0 | 1.7 |
| `tasks.tasks.list-fast` | 1.2 | 2.3 | 5.1 |
| `tasks.project` | 1.1 | 6.1 | 8.5 |
| `tasks.project.children` | 6.3 | 37.2 | 40.3 |
| `tasks.project.parent-candidates` | 1.4 | 2.7 | 4.6 |
| `tasks.task` | 0.9 | 1.3 | 1.1 |
| `tasks.task.time-entries` | 1.0 | 1.1 | 1.0 |
| `tasks.by-due-date` | 37.7 | 95.0 | 236.8 |
| `tasks.by-due-date.slack` | 36.9 | 90.4 | 236.7 |
| `tasks.time-entries.week` | 1.5 | 5.9 | 12.2 |
| `tasks.time-entries.active` | 0.5 | 0.5 | 0.6 |
| `tasks.time-entries.summary` | 0.5 | 2.0 | 4.4 |
| `tasks.time-entries.history.30d` | 2.4 | 10.3 | 20.9 |
| `tasks.time-entries.history.5y` | 4.2 | 41.3 | 100.8 |
| `plan.today` | 1.1 | 2.9 | 5.9 |
| `plan.range.2w` | 3.2 | 5.8 | 9.6 |
| `plan.commitments` | 0.5 | 0.6 | 0.7 |
| `capacity.30d` | 17.5 | 39.3 | 88.4 |
| `calendar.month` | 0.6 | 0.9 | 1.1 |
| `finance.accounts` | 0.6 | 0.6 | 0.7 |
| `finance.categories` | 0.7 | 0.8 | 0.9 |
| `finance.overview` | 2.5 | 5.3 | 10.3 |
| `finance.transactions.all` | 2.7 | 35.3 | 83.1 |
| `finance.transactions.month` | 1.3 | 3.2 | 5.9 |
| `finance.budgets` | 2.9 | 8.0 | 17.4 |
| `finance.stats.monthly.1y` | 0.6 | 2.3 | 5.8 |
| `finance.stats.monthly.5y` | 0.8 | 6.4 | 14.9 |
| `finance.stats.by-category.1y` | 1.3 | 3.0 | 7.0 |
| `finance.stats.networth.1y` | 7.5 | 70.8 | 237.9 |
| `finance.stats.networth.5y` | 8.7 | 92.1 | 292.1 |
| `finance.stats.estimation` | 1.9 | 32.2 | 129.6 |
| `rutas.marks` | 0.4 | 1.4 | 1.8 |
| `uptime` | 0.8 | 1.9 | 3.3 |
| `uptime.windows.30d` | 0.7 | 1.2 | 1.9 |
| `w.habits.log` | 17.8 | 32.2 | 37.7 |
| `w.finance.transaction` | 8.3 | 8.8 | 9.0 |
| `w.tasks.task` | 9.6 | 15.3 | 16.7 |
| `w.tasks.time-entry` | 7.5 | 8.0 | 7.9 |
| `w.plan.block` | 8.9 | 9.8 | 10.3 |
| `w.calendar.event` | 7.5 | 7.8 | 7.4 |

## Where the time goes

CPU per request, measured from `/proc` (API process and Postgres backends), 20 sequential requests:

| Endpoint | Scenario | Wall | Go CPU | Postgres CPU |
|---|---|---:|---:|---:|
| `habits.daily` | 5y | 57 | 0.5 | 55.5 |
| `tasks.by-due-date` | 5y / ×3 | 78 / 209 | 4.5 / 12 | 74.5 / 198.5 |
| `tasks.by-due-date.slack` | 5y / ×3 | 82 / 209 | 3.5 / 10 | 78 / 199.5 |
| `capacity.30d` | 5y | 33 | 2 | 30.5 |
| `finance.stats.networth.1y` | 5y | 61 | 1 | 59 |
| `finance.stats.estimation` | 5y | 26 | 0.5 | 24.5 |
| `tasks.time-entries.history.5y` | 5y | 34 | 0 | 33 |
| `tasks.project.children` | 5y | 24 | 1 | 22.5 |
| `finance.transactions.all` | 5y / ×3 | 28 / 67 | 34 / 86 | 6 / 15 |

## Process

| Scenario | Startup | Idle RSS | Peak RSS (under load) | Total CPU of the run |
|---|---:|---:|---:|---:|
| prod | 75 ms | 30 MB | 42 MB | 17 s |
| 5y | 73 ms | 28 MB | 83 MB | 53 s |
| 5y_x3 | 71 ms | 26 MB | 151 MB | 139 s |

## Findings

1. **`habits.daily` grows with history, not with the number of habits** (9 → 60 → 107 ms). The query does `LEFT JOIN LATERAL recalculate_habit_streak(...) ON h.target_max = 0`, and Postgres evaluates the function for **every** habit before applying the `ON` condition. That walks each habit's whole history on every request. `w.habits.log` (15 → 27 ms) also recalculates streaks.
2. **`by-due-date` and `slack` are the heaviest** (about 14 queries per request, SQL-bound) and grow with the number of open tasks and dependencies. gv-web calls both.
3. **`finance.stats.networth` and `estimation`** grow linearly with the transaction history (they rebuild the series from the start).
4. **gv-web downloads the whole transaction history**: `overview.svelte.ts:193` calls `listTransactions()` without filters. That is 1.1 MB at 5 years and 3.2 MB at ×3, plus the parsing in the browser.
5. **Writes** take 6–15 ms (27–32 ms for `habits.log` at 5y) and barely grow with volume.
6. **Memory and startup** are small: 26–30 MB RSS idle, 83–151 MB peak under concurrency at 5y and ×3, and about 70 ms to answer `/health`.

None of these are regressions or blockers. Fixing them changes queries or the contract (pagination), so they are left **out of the rewrite** (which requires parity) and noted as candidates for a later change.

## Implications for the rewrite

- The heavy endpoints are SQL-bound, so porting the same SQL keeps the latency. The real risk is limited to per-request framework overhead (around 1 ms, to be measured) and encoding large payloads, which was already measured with equivalent results.
- **Run-to-run noise:** repeating the `prod` scenario gave a median p95 ratio of 0.92, but single endpoints ranged from 0.40× to 1.53× (all of them under 15 ms). One run is not enough to compare.
- **Latency gate (replaces the 2× tasks/habits rule in design D15/Risks):** at scenario `5y`, for **every** endpoint, the median of 3 runs of Python p95 ≤ max(1.5 × median of 3 runs of Go p95, Go + 5 ms), measured with `bench/run.sh` on the same machine. An extra checkpoint at the end of H3 (task 12.3) catches problems before finance, printers and the rest.
