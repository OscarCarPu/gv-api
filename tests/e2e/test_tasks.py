"""Projects and tasks over HTTP against the running API."""

from datetime import UTC, datetime
from typing import Any

from tests.e2e.conftest import APIClient, truncate_tables


def ok(client: APIClient, method: str, path: str, body: Any = None, status: int = 200) -> Any:
    resp = client.do(method, path, body)
    assert resp.status_code == status, f"{method} {path}: got {resp.status_code}: {resp.text}"
    return resp.json()


def now() -> str:
    return datetime.now(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def test_project_and_task_lifecycle(authed: APIClient) -> None:
    truncate_tables()
    project = ok(authed, "POST", "/tasks/projects", {"name": "Project"}, 201)
    ok(authed, "PATCH", f"/tasks/projects/{project['id']}", {"started_at": now()})
    task = ok(authed, "POST", "/tasks/tasks", {"name": "Task", "project_id": project["id"]}, 201)
    ok(authed, "PATCH", f"/tasks/tasks/{task['id']}", {"started_at": now()})
    updated = ok(authed, "PATCH", f"/tasks/tasks/{task['id']}", {"finished_at": now()})
    assert updated["finished_at"] is not None
    assert ok(authed, "GET", f"/tasks/tasks/{task['id']}")["id"] == task["id"]


def test_move_project_parent(authed: APIClient) -> None:
    truncate_tables()
    root = ok(authed, "POST", "/tasks/projects", {"name": "Root"}, 201)
    child = ok(authed, "POST", "/tasks/projects", {"name": "Child", "parent_id": root["id"]}, 201)
    other = ok(authed, "POST", "/tasks/projects", {"name": "Other"}, 201)
    ok(authed, "PATCH", f"/tasks/projects/{child['id']}", {"parent_id": other["id"]})
    ok(authed, "PATCH", f"/tasks/projects/{other['id']}", {"parent_id": child["id"]}, 409)
    assert (
        ok(authed, "PATCH", f"/tasks/projects/{child['id']}", {"parent_id": None})["parent_id"]
        is None
    )
    candidates = ok(authed, "GET", f"/tasks/projects/{other['id']}/parent-candidates")
    assert other["id"] not in [c["id"] for c in candidates]


def test_task_dependency_chain(authed: APIClient) -> None:
    truncate_tables()
    a = ok(authed, "POST", "/tasks/tasks", {"name": "A"}, 201)
    b = ok(authed, "POST", "/tasks/tasks", {"name": "B", "depends_on": [a["id"]]}, 201)
    got = ok(authed, "GET", f"/tasks/tasks/{b['id']}")
    assert ([d["id"] for d in got["depends_on"]], got["blocked"]) == ([a["id"]], True)
    ok(authed, "PATCH", f"/tasks/tasks/{a['id']}", {"started_at": now(), "finished_at": now()})
    assert ok(authed, "GET", f"/tasks/tasks/{b['id']}")["blocked"] is False


def test_clear_due_at_via_patch(authed: APIClient) -> None:
    truncate_tables()
    task = ok(authed, "POST", "/tasks/tasks", {"name": "T", "due_at": "2026-06-01T00:00:00Z"}, 201)
    assert task["due_at"] == "2026-06-01T00:00:00Z"
    ok(authed, "PATCH", f"/tasks/tasks/{task['id']}", {"due_at": None})
    assert ok(authed, "GET", f"/tasks/tasks/{task['id']}")["due_at"] is None


def test_auth_required(client: APIClient) -> None:
    assert client.do("GET", "/tasks/projects").status_code == 401


def test_not_found(authed: APIClient) -> None:
    truncate_tables()
    assert authed.do("GET", "/tasks/tasks/999999").status_code == 404
