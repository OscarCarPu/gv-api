"""Measures gv-api endpoint latency over HTTP: one JSON line per endpoint on stdout.

uv run python bench/load.py -url http://127.0.0.1:18080 -secret <JWT_SECRET> [-only a,b]
"""

import argparse
import json
import sys
import time
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Any

import httpx
import jwt


@dataclass
class Endpoint:
    name: str
    path: str
    kind: str = "full"  # token kind; "" means none
    method: str = "GET"
    body: str = ""
    expect: int = 0  # accepted status besides 2xx
    delete_path: str = ""  # write cycle: POST, then DELETE this path with {id}


def token(secret: str, kind: str) -> str:
    now = int(time.time())
    return jwt.encode({"kind": kind, "iat": now, "exp": now + 3600}, secret, algorithm="HS256")


def pct(values: list[float], p: float) -> float:
    return values[min(int(p * len(values) + 0.5), len(values) - 1)]


def summary(lat: list[float]) -> dict[str, float]:
    lat = sorted(lat)
    return {
        "p50_ms": pct(lat, 0.50),
        "p95_ms": pct(lat, 0.95),
        "p99_ms": pct(lat, 0.99),
        "max_ms": lat[-1],
        "mean_ms": sum(lat) / len(lat),
    }


class Runner:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.http = httpx.Client(base_url=args.url, timeout=60)

    def do(self, e: Endpoint, method: str, path: str, body: str) -> tuple[int, bytes, float]:
        headers = {"Authorization": f"Bearer {token(self.args.secret, e.kind)}"} if e.kind else {}
        if body:
            headers["Content-Type"] = "application/json"
        start = time.perf_counter()
        resp = self.http.request(method, path, content=body or None, headers=headers)
        return resp.status_code, resp.content, (time.perf_counter() - start) * 1000

    def ok(self, status: int, e: Endpoint) -> bool:
        return status == e.expect or 200 <= status < 300

    def run(self, e: Endpoint) -> dict[str, Any]:
        out: dict[str, Any] = {"scenario": self.args.scenario, "name": e.name, "method": e.method}
        out |= {"path": e.path, "status": 0, "bytes": 0, "n": 0, "errors": 0}
        lat: list[float] = []
        dlat: list[float] = []
        warm, n = self.args.warm, self.args.n
        for i in range(warm + n):
            status, body, ms = self.do(e, e.method, e.path, e.body)
            out["status"], out["bytes"] = status, len(body)
            if not self.ok(status, e):
                out["errors"] += 1
                continue
            if i >= warm:
                lat.append(ms)
            if e.delete_path:
                id = json.loads(body).get("id")
                status, _, ms = self.do(e, "DELETE", e.delete_path.replace("{id}", str(id)), "")
                if not self.ok(status, e):
                    out["errors"] += 1
                elif i >= warm:
                    dlat.append(ms)
        if lat:
            out["n"] = len(lat)
            out |= summary(lat)
        if dlat:
            d = summary(dlat)
            out |= {"delete_p50_ms": d["p50_ms"], "delete_p95_ms": d["p95_ms"]}
        return out


def endpoints(task: int, project: int) -> list[Endpoint]:
    now = datetime.now()
    utc = datetime.now(UTC)

    def day(off: int) -> str:
        return (now + timedelta(days=off)).strftime("%Y-%m-%d")

    def ts(off: timedelta) -> str:
        return (utc + off).strftime("%Y-%m-%dT%H:%M:%SZ")

    def month(off: int) -> str:
        y, m = divmod(now.year * 12 + now.month - 1 + off, 12)
        return f"{y:04d}-{m + 1:02d}"

    t, p, d30 = str(task), str(project), timedelta(days=30)
    reads = [
        Endpoint("health", "/health", kind=""),
        Endpoint("habits.daily", f"/habits?date={day(0)}"),
        Endpoint(
            "habits.history.1y",
            f"/habits/1/history?frequency=daily&start_at={day(-365)}&end_at={day(0)}",
        ),
        Endpoint("tasks.tree", "/tasks/tree"),
        Endpoint("tasks.projects.roots", "/tasks/projects"),
        Endpoint("tasks.projects.list-fast", "/tasks/projects/list-fast"),
        Endpoint("tasks.tasks.list-fast", "/tasks/tasks/list-fast"),
        Endpoint("tasks.project", f"/tasks/projects/{p}"),
        Endpoint("tasks.project.children", f"/tasks/projects/{p}/children"),
        Endpoint("tasks.project.parent-candidates", f"/tasks/projects/{p}/parent-candidates"),
        Endpoint("tasks.task", f"/tasks/tasks/{t}"),
        Endpoint("tasks.task.time-entries", f"/tasks/tasks/{t}/time-entries"),
        Endpoint("tasks.by-due-date", "/tasks/tasks/by-due-date"),
        Endpoint("tasks.by-due-date.slack", "/tasks/tasks/by-due-date/slack"),
        Endpoint("tasks.time-entries.week", f"/tasks/time-entries?start_time={day(-7)}"),
        Endpoint("tasks.time-entries.active", "/tasks/time-entries/active", expect=404),
        Endpoint("tasks.time-entries.summary", "/tasks/time-entries/summary"),
        Endpoint(
            "tasks.time-entries.history.30d",
            f"/tasks/time-entries/history?frequency=daily&start_at={day(-30)}&end_at={day(0)}",
        ),
        Endpoint(
            "tasks.time-entries.history.5y",
            f"/tasks/time-entries/history?frequency=monthly&start_at={day(-1900)}&end_at={day(0)}",
        ),
        Endpoint("plan.today", "/plan/today"),
        Endpoint("plan.range.2w", f"/plan/range?from={day(-7)}&to={day(7)}"),
        Endpoint("plan.commitments", "/plan/commitments"),
        Endpoint("capacity.30d", f"/capacity/free-busy?from={day(0)}&to={day(30)}"),
        Endpoint(
            "calendar.month",
            f"/calendar/events?from={ts(-d30 / 2)}&to={ts(d30 / 2)}",
        ),
        Endpoint("finance.accounts", "/finance/accounts"),
        Endpoint("finance.categories", "/finance/categories"),
        Endpoint("finance.overview", "/finance/overview"),
        Endpoint("finance.transactions.all", "/finance/transactions"),
        Endpoint(
            "finance.transactions.month", f"/finance/transactions?from={day(-30)}&to={day(0)}"
        ),
        Endpoint("finance.transactions.account", "/finance/transactions?account_id=1"),
        Endpoint("finance.budgets", f"/finance/budgets?month={month(0)}"),
        Endpoint(
            "finance.stats.monthly.1y", f"/finance/stats/monthly?from={day(-365)}&to={day(0)}"
        ),
        Endpoint(
            "finance.stats.monthly.5y", f"/finance/stats/monthly?from={day(-1900)}&to={day(0)}"
        ),
        Endpoint(
            "finance.stats.by-category.1y",
            f"/finance/stats/by-category?type=expense&from={day(-365)}&to={day(0)}",
        ),
        Endpoint(
            "finance.stats.networth.1y",
            f"/finance/stats/networth?from={day(-365)}&to={day(0)}&granularity=week",
        ),
        Endpoint(
            "finance.stats.networth.5y",
            f"/finance/stats/networth?from={day(-1900)}&to={day(0)}&granularity=month",
        ),
        Endpoint(
            "finance.stats.estimation",
            f"/finance/stats/estimation?start_month={month(-12)}&end_month={month(0)}&mode=rate",
        ),
        Endpoint("rutas.marks", "/rutas/marks", kind="semi"),
        Endpoint("uptime", "/domotics/uptime", kind="semi"),
        Endpoint(
            "uptime.windows.30d",
            f"/domotics/uptime/windows?from={ts(-d30)}&to={ts(timedelta())}",
            kind="semi",
        ),
    ]
    start, end = ts(-timedelta(hours=3)), ts(-timedelta(hours=2))
    # Plan blocks reject overlaps; this slot is past every seeded block.
    b_start, b_end = ts(timedelta(days=700)), ts(timedelta(days=700, hours=1))
    writes = [
        Endpoint(
            "w.habits.log",
            "/habits/log",
            method="POST",
            body=json.dumps({"habit_id": 1, "date": day(0), "value": 2}),
        ),
        Endpoint(
            "w.finance.transaction",
            "/finance/transactions",
            method="POST",
            delete_path="/finance/transactions/{id}",
            body=json.dumps(
                {
                    "type": "expense",
                    "amount": "12.34",
                    "account_id": 1,
                    "category_id": 4,
                    "description": "bench",
                }
            ),
        ),
        Endpoint(
            "w.tasks.task",
            "/tasks/tasks",
            method="POST",
            delete_path="/tasks/tasks/{id}",
            body=json.dumps({"name": "bench", "project_id": project}),
        ),
        Endpoint(
            "w.tasks.time-entry",
            "/tasks/time-entries",
            method="POST",
            delete_path="/tasks/time-entries/{id}",
            body=json.dumps({"task_id": task, "started_at": start, "finished_at": end}),
        ),
        Endpoint(
            "w.plan.block",
            "/plan/blocks",
            method="POST",
            delete_path="/plan/blocks/{id}",
            body=json.dumps({"started_at": b_start, "ended_at": b_end, "label": "bench"}),
        ),
        Endpoint(
            "w.calendar.event",
            "/calendar/events",
            method="POST",
            delete_path="/calendar/events/{id}",
            body=json.dumps({"name": "bench", "start_time": start, "end_time": end}),
        ),
    ]
    return reads + writes


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-url", default="http://localhost:18080")
    parser.add_argument("-secret", required=True)
    parser.add_argument("-scenario", default="")
    parser.add_argument("-n", type=int, default=100, help="measured requests per endpoint")
    parser.add_argument("-warm", type=int, default=10, help="warm-up requests per endpoint")
    parser.add_argument("-task", type=int, default=1, help="id of an open task")
    parser.add_argument("-project", type=int, default=1, help="id of a root project")
    parser.add_argument("-only", default="", help="comma-separated endpoint names")
    args = parser.parse_args()
    want = {s for s in args.only.split(",") if s}
    runner = Runner(args)
    for e in endpoints(args.task, args.project):
        if want and e.name not in want:
            continue
        r = runner.run(e)
        print(json.dumps(r), flush=True)
        print(
            f"{r['name']:34s} {r['status']:3d} p50 {r.get('p50_ms', 0):7.2f}"
            f" p95 {r.get('p95_ms', 0):7.2f} ms  {r['bytes']:7d} B  err {r['errors']}",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
