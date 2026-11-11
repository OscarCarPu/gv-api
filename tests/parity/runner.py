"""Parity runner (design D15): replays the corpus against the Go release and the Python app.

    uv run python -m tests.parity.runner [corpus-name ...]     (or `make parity`)

Each request goes to both apps at the same time; status, selected headers and decoded bodies
must match, ignoring the entry's volatile paths. After each corpus file both databases must hold
the same rows, ignoring the volatile columns. Exits 1 on any difference.
"""

import asyncio
import difflib
import json
import os
import re
import statistics
import sys
import time
from collections import Counter
from collections.abc import Iterable, Sequence
from dataclasses import dataclass, field
from datetime import datetime
from decimal import Decimal
from pathlib import Path
from typing import Any, Literal
from zoneinfo import ZoneInfo

import asyncpg
import httpx
import pyotp
import yaml
from pydantic import BaseModel, ConfigDict

HERE = Path(__file__).parent
CORPUS_DIR = HERE / "corpus"
ACCEPTED = HERE / "ACCEPTED.md"
RESULTS = HERE / "results"

# Compared when either app sends them; Date, Content-Length and X-Request-Id always differ.
HEADERS = (
    "content-type",
    "allow",
    "location",
    "content-disposition",
    "content-range",
    "accept-ranges",
    "cache-control",
    "x-content-type-options",
    "vary",
    "access-control-allow-origin",
    "access-control-allow-methods",
    "access-control-allow-headers",
    "access-control-allow-credentials",
    "access-control-expose-headers",
    "access-control-max-age",
)
VOLATILE = "<volatile>"
_VAR = re.compile(r"\{\{\s*([\w.]+)\s*\}\}")
_TOKEN = re.compile(r"\[(\*|\d+)\]|([^.\[\]]+)")


class _Model(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class Expect(_Model):
    status: int
    body: Any = None
    text: str | None = None


class Entry(_Model):
    name: str
    method: str = "GET"
    path: str
    tier: Literal["public", "semi", "full"] = "public"
    headers: dict[str, str] = {}
    body: Any = None
    raw: str | None = None
    volatile: list[str] = []
    volatile_headers: list[str] = []
    # A generated binary body (a dump): only the status and headers are compared.
    opaque: bool = False
    capture: dict[str, str] = {}
    # A known difference: Python is checked against expect_py instead of against Go.
    accepted: str | None = None
    expect_py: Expect | None = None


class Corpus(_Model):
    db_volatile: list[str] = []  # table.column or *.column
    db_ignore: list[str] = []  # whole tables
    requests: list[Entry] = []


# JSON paths: `a.b`, `items[*].id`, `[0].id`, `*` (any key or index), `**` (any depth).


def parse_path(pattern: str) -> list[str]:
    tokens: list[str] = []
    for index, key in _TOKEN.findall(pattern):
        tokens.append(f"[{index}]" if index else key)
    return tokens


def _matches(pattern: Sequence[str], path: Sequence[str | int]) -> bool:
    if not pattern:
        return not path
    head, rest = pattern[0], pattern[1:]
    if head == "**":
        return _matches(rest, path) or (bool(path) and _matches(pattern, path[1:]))
    if not path:
        return False
    seg = path[0]
    if head == "*":
        ok = True
    elif head == "[*]":
        ok = isinstance(seg, int)
    elif head.startswith("["):
        ok = isinstance(seg, int) and seg == int(head[1:-1])
    else:
        ok = isinstance(seg, str) and seg == head
    return ok and _matches(rest, path[1:])


def mask(value: Any, patterns: Iterable[str], path: tuple[str | int, ...] = ()) -> Any:
    parsed = [parse_path(p) for p in patterns]
    return _mask(value, parsed, path)


def _mask(value: Any, patterns: list[list[str]], path: tuple[str | int, ...]) -> Any:
    if path and any(_matches(p, path) for p in patterns):
        return VOLATILE
    if isinstance(value, dict):
        return {k: _mask(v, patterns, (*path, k)) for k, v in value.items()}
    if isinstance(value, list):
        return [_mask(v, patterns, (*path, i)) for i, v in enumerate(value)]
    return value


def extract(value: Any, pattern: str) -> Any:
    for token in parse_path(pattern):
        if token.startswith("["):
            if not isinstance(value, list):
                raise KeyError(pattern)
            value = value[int(token[1:-1])]
        else:
            if not isinstance(value, dict) or token not in value:
                raise KeyError(pattern)
            value = value[token]
    return value


@dataclass
class Target:
    name: str
    client: httpx.AsyncClient
    vars: dict[str, str] = field(default_factory=dict)
    tokens: dict[str, str] = field(default_factory=dict)

    def substitute(self, value: Any) -> Any:
        if isinstance(value, str):
            return _VAR.sub(lambda m: self._var(m.group(1)), value)
        if isinstance(value, dict):
            return {self.substitute(k): self.substitute(v) for k, v in value.items()}
        if isinstance(value, list):
            return [self.substitute(v) for v in value]
        return value

    def _var(self, name: str) -> str:
        if name == "totp":
            return pyotp.TOTP(self.vars["totp_secret"]).now()
        if name.startswith("token."):
            return self.tokens[name.removeprefix("token.")]
        if name not in self.vars:
            raise KeyError(f"unknown variable {{{{{name}}}}}")
        return self.vars[name]


@dataclass
class Reply:
    status: int
    headers: dict[str, list[str]]
    body: Any
    is_json: bool
    ms: float


async def send(target: Target, entry: Entry) -> Reply:
    headers = {}
    if entry.tier != "public":
        headers["Authorization"] = f"Bearer {target.tokens[entry.tier]}"
    headers |= target.substitute(entry.headers)
    content: bytes | None = None
    if entry.raw is not None:
        content = target.substitute(entry.raw).encode()
    elif entry.body is not None:
        content = json.dumps(target.substitute(entry.body)).encode()
        headers.setdefault("Content-Type", "application/json")
    start = time.perf_counter()
    resp = await target.client.request(
        entry.method, target.substitute(entry.path), headers=headers, content=content
    )
    ms = (time.perf_counter() - start) * 1000
    picked = {h: resp.headers.get_list(h) for h in HEADERS if h in resp.headers}
    body: Any = resp.text
    is_json = resp.headers.get("content-type", "").startswith("application/json")
    if is_json:
        try:
            body = resp.json()
        except ValueError:
            is_json = False
    return Reply(resp.status_code, picked, body, is_json, ms)


def _dump(value: Any) -> list[str]:
    if isinstance(value, str):
        return value.splitlines() or [""]
    return json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False).splitlines()


def body_diff(left: Any, right: Any, names: tuple[str, str] = ("go", "py")) -> list[str]:
    lines = difflib.unified_diff(_dump(left), _dump(right), names[0], names[1], lineterm="")
    return list(lines)


# chi builds Allow from a Go map, so its order changes from one request to the next.
UNORDERED = ("allow",)


def _header(reply: Reply, name: str) -> list[str] | None:
    values = reply.headers.get(name)
    if values is not None and name in UNORDERED:
        return sorted(values)
    return values


# framework-errors in ACCEPTED.md: Python answers unknown paths and methods with FastAPI's JSON.
FRAMEWORK_ERRORS = {404: {"detail": "Not Found"}, 405: {"detail": "Method Not Allowed"}}


def _allowed(reply: Reply) -> set[str]:
    return {m.strip() for value in reply.headers.get("allow", []) for m in value.split(",")}


def drop_absent(go: Any, py: Any) -> Any:
    """always-present-fields in ACCEPTED.md: Python sends null or [] for keys Go left out."""
    if isinstance(go, dict) and isinstance(py, dict):
        return {
            k: drop_absent(go.get(k), v) for k, v in py.items() if k in go or v not in (None, [])
        }
    if isinstance(go, list) and isinstance(py, list):
        return [drop_absent(go[i] if i < len(go) else None, v) for i, v in enumerate(py)]
    return py


_AMOUNT = re.compile(r"-?\d+(\.\d+)?")
_INSTANT = re.compile(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)")


def seven_digits(value: Any) -> Any:
    """real-seven-digits in ACCEPTED.md: floats compare at 7 significant digits.

    native-encoding: amount strings compare by value ("12.50" == "12.5") and instants by the
    moment they name ("…:05.120000Z" == "…:05.12Z").
    """
    if isinstance(value, float):
        return float(f"{value:.7g}")
    if isinstance(value, str):
        if _AMOUNT.fullmatch(value):
            return f"amount:{Decimal(value).normalize():f}"
        if _INSTANT.fullmatch(value):
            return datetime.fromisoformat(value.replace("Z", "+00:00")).isoformat()
        return value
    if isinstance(value, dict):
        return {k: seven_digits(v) for k, v in value.items()}
    if isinstance(value, list):
        return [seven_digits(v) for v in value]
    return value


def compare(entry: Entry, go: Reply, py: Reply) -> list[str]:
    problems = []
    if go.status == py.status in FRAMEWORK_ERRORS and (
        py.body == FRAMEWORK_ERRORS[py.status] or entry.method == "HEAD"  # HEAD has no body
    ):
        if _allowed(go) != _allowed(py):
            problems.append(f"header allow: go={sorted(_allowed(go))} py={sorted(_allowed(py))}")
        return problems
    if go.status != py.status:
        problems.append(f"status: go={go.status} py={py.status}")
    for h in sorted((set(go.headers) | set(py.headers)) - set(entry.volatile_headers)):
        if _header(go, h) != _header(py, h):
            problems.append(f"header {h}: go={go.headers.get(h)} py={py.headers.get(h)}")
    if entry.opaque:
        return problems
    left, right = go.body, py.body
    if go.is_json and py.is_json:
        left, right = mask(left, entry.volatile), mask(right, entry.volatile)
        right = drop_absent(left, right)
        left, right = seven_digits(left), seven_digits(right)
    if left != right or go.is_json != py.is_json:
        problems.append("body:")
        problems += ["  " + line for line in body_diff(left, right)]
    return problems


def check_expected(entry: Entry, py: Reply) -> list[str]:
    expect = entry.expect_py
    if expect is None:
        return ["accepted entry without expect_py"]
    problems = []
    if py.status != expect.status:
        problems.append(f"status: expected={expect.status} py={py.status}")
    if expect.body is not None and mask(py.body, entry.volatile) != expect.body:
        problems += ["body:"] + [
            "  " + x for x in body_diff(expect.body, py.body, ("expected", "py"))
        ]
    if expect.text is not None and py.body != expect.text:
        problems.append(f"text: expected={expect.text!r} py={py.body!r}")
    return problems


@dataclass
class Outcome:
    corpus: str
    index: int
    entry: Entry
    problems: list[str]
    go_ms: float
    py_ms: float


def accepted_ids() -> set[str]:
    if not ACCEPTED.exists():
        return set()
    return set(re.findall(r"^## (\S+)", ACCEPTED.read_text(), re.MULTILINE))


async def run_corpus(name: str, corpus: Corpus, go: Target, py: Target) -> list[Outcome]:
    known = accepted_ids()
    outcomes = []
    for i, entry in enumerate(corpus.requests, 1):
        try:
            go_reply, py_reply = await asyncio.gather(send(go, entry), send(py, entry))
        except (httpx.HTTPError, KeyError) as err:
            problems = [f"request failed: {type(err).__name__}: {err}"]
            outcomes.append(Outcome(name, i, entry, problems, 0, 0))
            continue
        if entry.accepted is not None:
            problems = check_expected(entry, py_reply)
            if entry.accepted not in known:
                problems.append(f"accepted id {entry.accepted!r} is not a heading in ACCEPTED.md")
        else:
            problems = compare(entry, go_reply, py_reply)
        for var, path in entry.capture.items():
            for target, reply in ((go, go_reply), (py, py_reply)):
                try:
                    target.vars[var] = str(extract(reply.body, path))
                except KeyError, IndexError:
                    problems.append(f"capture {var}: {path!r} missing in {target.name}")
        outcomes.append(Outcome(name, i, entry, problems, go_reply.ms, py_reply.ms))
    return outcomes


# Database comparison


def _volatile_columns(patterns: Iterable[str], table: str) -> set[str]:
    out = set()
    for pattern in patterns:
        t, _, column = pattern.partition(".")
        if t in ("*", table):
            out.add(column)
    return out


def diff_tables(
    go: dict[str, list[dict[str, Any]]],
    py: dict[str, list[dict[str, Any]]],
    volatile: Iterable[str],
    ignore: Iterable[str] = (),
    limit: int = 5,
) -> list[str]:
    problems = []
    patterns = list(volatile)
    for table in sorted((set(go) | set(py)) - set(ignore)):
        if table not in go or table not in py:
            problems.append(f"table {table}: only in {'go' if table in go else 'py'}")
            continue
        drop = _volatile_columns(patterns, table)

        def rows(data: list[dict[str, Any]], drop: set[str] = drop) -> Counter[str]:
            return Counter(
                json.dumps(
                    {k: v for k, v in r.items() if k not in drop}, sort_keys=True, default=str
                )
                for r in data
            )

        left, right = rows(go[table]), rows(py[table])
        if left == right:
            continue
        only_go, only_py = left - right, right - left
        problems.append(
            f"table {table}: {sum(only_go.values())} rows only in go, "
            f"{sum(only_py.values())} only in py"
        )
        problems += [f"  go: {r}" for r in list(only_go)[:limit]]
        problems += [f"  py: {r}" for r in list(only_py)[:limit]]
    return problems


async def snapshot(url: str) -> dict[str, list[dict[str, Any]]]:
    conn = await asyncpg.connect(url)
    try:
        tables = await conn.fetch(
            "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY 1"
        )
        out = {}
        for (table,) in tables:
            rows = await conn.fetch(f'SELECT to_jsonb(t)::text FROM public."{table}" t')  # noqa: S608
            out[table] = [json.loads(r[0]) for r in rows]
        return out
    finally:
        await conn.close()


# Command line


def load_env(path: Path) -> dict[str, str]:
    env = {}
    for line in path.read_text().splitlines():
        if line.strip() and not line.startswith("#"):
            key, _, value = line.partition("=")
            env[key.strip()] = value.strip()
    return env


async def authenticate(target: Target) -> None:
    async def post(path: str, body: dict[str, str]) -> str:
        resp = await target.client.post(path, json=body)
        resp.raise_for_status()
        return str(resp.json()["token"])

    tmp = await post("/login", {"password": target.vars["password"]})
    target.tokens["tmp"] = tmp
    target.tokens["semi"] = await post("/login", {"password": target.vars["semi_password"]})
    code = pyotp.TOTP(target.vars["totp_secret"]).now()
    target.tokens["full"] = await post("/login/2fa", {"token": tmp, "code": code})


def load_corpora(names: Sequence[str]) -> dict[str, Corpus]:
    files = [CORPUS_DIR / f"{n.removesuffix('.yaml')}.yaml" for n in names] or sorted(
        CORPUS_DIR.glob("*.yaml")
    )
    return {f.stem: Corpus.model_validate(yaml.safe_load(f.read_text())) for f in files}


def report(outcomes: list[Outcome], out: Any = sys.stdout) -> int:
    failed = [o for o in outcomes if o.problems]
    for o in failed:
        e = o.entry
        print(f"\nFAIL {o.corpus} #{o.index} {e.name!r}: {e.method} {e.path}", file=out)
        for line in o.problems:
            print(f"  {line}", file=out)
    timed = [o for o in outcomes if o.go_ms]
    if timed:
        go = statistics.median(o.go_ms for o in timed)
        py = statistics.median(o.py_ms for o in timed)
        print(f"\nlatency median: go {go:.1f} ms, py {py:.1f} ms", file=out)
    accepted = sum(1 for o in outcomes if o.entry.accepted and not o.problems)
    print(
        f"{len(outcomes) - len(failed)}/{len(outcomes)} requests equal"
        + (f" ({accepted} accepted deviations)" if accepted else ""),
        file=out,
    )
    return 1 if failed else 0


async def main(argv: Sequence[str]) -> int:
    env = load_env(HERE / "parity.env")
    tz = ZoneInfo(env["TIMEZONE"])
    base_vars = {
        "password": env["PASSWORD"],
        "semi_password": env["SEMIPRIVATE_PASSWORD"],
        "totp_secret": env["TOTP_SECRET"],
        "today": datetime.now(tz).date().isoformat(),
    }
    db = "postgresql://parity:parity@127.0.0.1:15432/"
    corpora = load_corpora(argv)
    # Database settings apply to every run, so a partial run still knows the worker tables.
    every = load_corpora([]) | corpora
    db_volatile = [p for c in every.values() for p in c.db_volatile]
    db_ignore = [t for c in every.values() for t in c.db_ignore]

    outcomes: list[Outcome] = []
    db_problems: list[str] = []
    async with (
        httpx.AsyncClient(base_url=os.environ.get("PARITY_GO_URL", "http://127.0.0.1:18180")) as gc,
        httpx.AsyncClient(base_url=os.environ.get("PARITY_PY_URL", "http://127.0.0.1:18181")) as pc,
    ):
        go, py = Target("go", gc, dict(base_vars)), Target("py", pc, dict(base_vars))
        await asyncio.gather(authenticate(go), authenticate(py))
        for name, corpus in corpora.items():
            outcomes += await run_corpus(name, corpus, go, py)
            go_db, py_db = await asyncio.gather(
                snapshot(db + "parity_go"), snapshot(db + "parity_py")
            )
            db_problems += [
                f"after {name}: {p}" for p in diff_tables(go_db, py_db, db_volatile, db_ignore)
            ]

    RESULTS.mkdir(exist_ok=True)
    with (RESULTS / "latest.jsonl").open("w") as f:
        for o in outcomes:
            row: dict[str, object] = {
                "corpus": o.corpus,
                "name": o.entry.name,
                "path": o.entry.path,
            }
            row |= {"go_ms": round(o.go_ms, 2), "py_ms": round(o.py_ms, 2), "ok": not o.problems}
            f.write(json.dumps(row) + "\n")

    code = report(outcomes)
    if db_problems:
        print("\nDATABASE DIFF", *db_problems, sep="\n")
        code = 1
    return code


if __name__ == "__main__":
    sys.exit(asyncio.run(main(sys.argv[1:])))
