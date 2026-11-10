import io
from typing import Any

import httpx
import pytest
from fastapi import FastAPI, Request
from starlette.responses import JSONResponse, PlainTextResponse, Response

from tests.parity import runner
from tests.parity.runner import Corpus, Target, diff_tables, mask, report, run_corpus


def fake_app(broken: bool) -> FastAPI:
    app = FastAPI()
    store: dict[int, dict[str, Any]] = {}

    @app.post("/items")
    async def create(request: Request) -> Response:
        body = await request.json()
        item_id = len(store) + 7
        store[item_id] = {"id": item_id, "name": body["name"], "created_at": f"t{id(app)}"}
        return JSONResponse(store[item_id], status_code=201)

    @app.get("/items/{item_id}")
    async def get(item_id: int, request: Request) -> Response:
        if request.headers.get("authorization") != "Bearer full-token":
            return JSONResponse({"error": "Unauthorized"}, status_code=401)
        item = dict(store[item_id])
        if broken:
            item["name"] = item["name"].upper()
        return JSONResponse(item)

    @app.get("/gone")
    async def gone() -> Response:
        if broken:
            return PlainTextResponse("404 page not found\n", status_code=404)
        return JSONResponse([])

    return app


def target(name: str, broken: bool) -> Target:
    client = httpx.AsyncClient(
        transport=httpx.ASGITransport(app=fake_app(broken)), base_url="http://t"
    )
    return Target(name, client, vars={}, tokens={"full": "full-token"})


CORPUS = Corpus.model_validate(
    {
        "requests": [
            {
                "name": "create",
                "method": "POST",
                "path": "/items",
                "body": {"name": "lamp"},
                "volatile": ["created_at"],
                "capture": {"item": "id"},
            },
            {
                "name": "read back",
                "path": "/items/{{item}}",
                "tier": "full",
                "volatile": ["created_at"],
            },
        ]
    }
)


async def test_identical_apps_pass_with_volatile_fields_masked() -> None:
    outcomes = await run_corpus("items", CORPUS, target("go", False), target("py", False))
    assert [o.problems for o in outcomes] == [[], []]
    out = io.StringIO()
    assert report(outcomes, out) == 0
    assert "2/2 requests equal" in out.getvalue()


async def test_broken_python_response_fails_with_a_readable_diff() -> None:
    outcomes = await run_corpus("items", CORPUS, target("go", False), target("py", True))
    out = io.StringIO()
    assert report(outcomes, out) == 1
    text = out.getvalue()
    assert "FAIL items #2 'read back': GET /items/{{item}}" in text
    assert '-  "name": "lamp"' in text
    assert '+  "name": "LAMP"' in text
    assert '"created_at": "<volatile>"' in text
    assert "1/2 requests equal" in text


async def test_status_and_header_differences_are_reported() -> None:
    corpus = Corpus.model_validate({"requests": [{"name": "gone", "path": "/gone"}]})
    (outcome,) = await run_corpus("x", corpus, target("go", False), target("py", True))
    assert "status: go=200 py=404" in outcome.problems
    assert any(
        p.startswith("header content-type: go=['application/json']") for p in outcome.problems
    )
    assert "  +404 page not found" in outcome.problems


def test_allow_order_is_ignored_but_its_methods_are_not() -> None:
    entry = runner.Entry(name="x", path="/x")

    def reply(*allow: str) -> runner.Reply:
        return runner.Reply(405, {"allow": list(allow)}, "", False, 0)

    assert runner.compare(entry, reply("PUT", "DELETE"), reply("DELETE", "PUT")) == []
    assert runner.compare(entry, reply("PUT", "DELETE"), reply("PUT")) == [
        "header allow: go=['PUT', 'DELETE'] py=['PUT']"
    ]


async def test_accepted_deviation_checks_python_against_its_expectation(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Any
) -> None:
    accepted = tmp_path / "ACCEPTED.md"
    accepted.write_text("# Accepted\n\n## gone-route\n\nWhy.\n")
    monkeypatch.setattr(runner, "ACCEPTED", accepted)
    entry = {"name": "gone", "path": "/gone", "accepted": "gone-route"}
    ok = {**entry, "expect_py": {"status": 404, "text": "404 page not found\n"}}
    wrong = {**entry, "expect_py": {"status": 200}}
    unknown = {**ok, "accepted": "not-listed"}
    corpus = Corpus.model_validate({"requests": [ok, wrong, unknown]})
    outcomes = await run_corpus("x", corpus, target("go", False), target("py", True))
    assert outcomes[0].problems == []
    assert outcomes[1].problems == ["status: expected=200 py=404"]
    assert outcomes[2].problems == ["accepted id 'not-listed' is not a heading in ACCEPTED.md"]


async def test_missing_capture_is_a_problem() -> None:
    corpus = Corpus.model_validate(
        {"requests": [{"name": "gone", "path": "/gone", "capture": {"id": "[0].id"}}]}
    )
    (outcome,) = await run_corpus("x", corpus, target("go", False), target("py", False))
    assert outcome.problems == [
        "capture id: '[0].id' missing in go",
        "capture id: '[0].id' missing in py",
    ]


def test_unknown_variable_is_an_error() -> None:
    t = Target("go", httpx.AsyncClient(), vars={"a": "1"})
    assert t.substitute({"x": ["{{a}}", "{{ a }}/b"]}) == {"x": ["1", "1/b"]}
    with pytest.raises(KeyError, match="unknown variable"):
        t.substitute("{{nope}}")


@pytest.mark.parametrize(
    ("patterns", "expected"),
    [
        (
            ["a.created_at"],
            {"a": {"created_at": "<volatile>", "id": 1}, "items": [{"t": 1}, {"t": 2}]},
        ),
        (["items[*].t"], {"a": {"created_at": 5, "id": 1}, "items": [{"t": "<volatile>"}] * 2}),
        (
            ["items[1].t"],
            {"a": {"created_at": 5, "id": 1}, "items": [{"t": 1}, {"t": "<volatile>"}]},
        ),
        (["*.id"], {"a": {"created_at": 5, "id": "<volatile>"}, "items": [{"t": 1}, {"t": 2}]}),
        (["**.t"], {"a": {"created_at": 5, "id": 1}, "items": [{"t": "<volatile>"}] * 2}),
        (["missing.path"], {"a": {"created_at": 5, "id": 1}, "items": [{"t": 1}, {"t": 2}]}),
    ],
)
def test_mask_paths(patterns: list[str], expected: dict[str, Any]) -> None:
    value = {"a": {"created_at": 5, "id": 1}, "items": [{"t": 1}, {"t": 2}]}
    assert mask(value, patterns) == expected


def test_diff_tables() -> None:
    go: dict[str, list[dict[str, Any]]] = {
        "habits": [
            {"id": 1, "name": "a", "created_at": "x"},
            {"id": 2, "name": "b", "created_at": "x"},
        ],
        "outages": [{"id": 1}],
        "only_go": [],
    }
    py: dict[str, list[dict[str, Any]]] = {
        "habits": [
            {"id": 2, "name": "b", "created_at": "y"},
            {"id": 1, "name": "a", "created_at": "y"},
        ],
        "outages": [],
        "only_go_not": [],
    }
    assert diff_tables(go, py, ["*.created_at"], ignore=["outages", "only_go", "only_go_not"]) == []
    problems = diff_tables(go, py, ["habits.created_at"])
    assert "table only_go: only in go" in problems
    assert "table outages: 1 rows only in go, 0 only in py" in problems
    assert '  go: {"id": 1}' in problems
    py["habits"][0]["name"] = "B"
    assert diff_tables(go, py, ["*.created_at"], ignore=["outages", "only_go", "only_go_not"]) == [
        "table habits: 1 rows only in go, 1 only in py",
        '  go: {"id": 2, "name": "b"}',
        '  py: {"id": 2, "name": "B"}',
    ]


def test_every_corpus_file_parses() -> None:
    corpora = runner.load_corpora([])
    assert {"auth", "health", "http", "lights"} <= set(corpora)
    ids = runner.accepted_ids()
    for corpus in corpora.values():
        for entry in corpus.requests:
            assert entry.accepted is None or entry.accepted in ids
