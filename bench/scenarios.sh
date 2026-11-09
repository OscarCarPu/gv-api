# Data scales for bench/seed.sql. Sourced by bench/run.sh.
#
# prod: production row counts on 2026-11-09 (about 75 days of use since 2026-08-27).
# 5y:   prod plus 60 months at the monthly rates observed in production.
# 5y-x3: prod plus 60 months at three times those rates, as headroom.
#
# Monthly rates seen in production: ~80 transactions, ~215 time entries, ~27 habit logs,
# ~50 tasks, ~48 todos, ~75 plan blocks, ~6 outage windows.

scenario_prod() {
  days=75 n_accounts=10 n_cat_roots=18 n_cat_children=12 n_tx=173 n_budget_cats=3 budget_months=3
  n_habits=5 n_hl=67 n_projects=30 n_proj_roots=10 n_tasks=188 n_open=97 n_dep=72 n_todos=121
  n_te=501 n_pb=616 future_days=570 n_ev=662 ev_days_back=3300 n_marks=8 n_out=5
}

scenario_5y() {
  days=1900 n_accounts=15 n_cat_roots=22 n_cat_children=23 n_tx=5000 n_budget_cats=8 budget_months=62
  n_habits=10 n_hl=1700 n_projects=150 n_proj_roots=40 n_tasks=3200 n_open=200 n_dep=1200 n_todos=3000
  n_te=13400 n_pb=5100 future_days=570 n_ev=1900 ev_days_back=5100 n_marks=188 n_out=365
}

scenario_5y_x3() {
  days=1900 n_accounts=20 n_cat_roots=30 n_cat_children=45 n_tx=14600 n_budget_cats=20 budget_months=62
  n_habits=15 n_hl=5000 n_projects=400 n_proj_roots=100 n_tasks=9200 n_open=600 n_dep=3500 n_todos=8800
  n_te=39200 n_pb=14000 future_days=570 n_ev=4300 ev_days_back=5100 n_marks=313 n_out=1081
}
