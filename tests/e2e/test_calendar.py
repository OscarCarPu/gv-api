"""Calendar events over HTTP against the running API."""

from datetime import datetime

from tests.e2e.conftest import APIClient, truncate_tables


def test_requires_auth(client: APIClient) -> None:
    resp = client.do("GET", "/calendar/events?from=2026-08-20&to=2026-08-21")
    assert resp.status_code == 401


def test_event_lifecycle(authed: APIClient) -> None:
    truncate_tables()
    resp = authed.do(
        "POST",
        "/calendar/events",
        {
            "name": "Dentist",
            "start_time": "2026-08-20T09:00:00Z",
            "end_time": "2026-08-20T10:00:00Z",
        },
    )
    assert resp.status_code == 201
    created = resp.json()
    path = f"/calendar/events/{created['id']}"

    listed = authed.do("GET", "/calendar/events?from=2026-08-20&to=2026-08-21").json()
    assert [e["name"] for e in listed] == ["Dentist"]
    assert authed.do("GET", "/calendar/events?from=2026-08-21&to=2026-08-22").json() == []

    resp = authed.do("PATCH", path, {"name": "Dentist (moved)", "end_time": "2026-08-20T11:00:00Z"})
    assert resp.status_code == 200
    updated = resp.json()
    assert updated["name"] == "Dentist (moved)"
    assert datetime.fromisoformat(updated["end_time"]) == datetime.fromisoformat(
        "2026-08-20T11:00:00+00:00"
    )

    assert authed.do("DELETE", path).status_code == 204
    assert authed.do("GET", path).status_code == 404


def test_validation(authed: APIClient) -> None:
    truncate_tables()
    for method, path, body in [
        (
            "POST",
            "/calendar/events",
            {"start_time": "2026-08-20T09:00:00Z", "end_time": "2026-08-20T10:00:00Z"},
        ),
        (
            "POST",
            "/calendar/events",
            {"name": "x", "start_time": "2026-08-20T10:00:00Z", "end_time": "2026-08-20T09:00:00Z"},
        ),
        ("GET", "/calendar/events", None),
        ("GET", "/calendar/events?from=2026-08-21&to=2026-08-20", None),
    ]:
        assert authed.do(method, path, body).status_code == 400, f"{method} {path} {body}"
    assert authed.do("PATCH", "/calendar/events/999", {"name": "x"}).status_code == 404
