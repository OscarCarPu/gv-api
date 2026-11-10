"""From test/e2e/habits_test.go."""

from tests.e2e.conftest import APIClient, truncate_tables


def test_insert_log_and_verify_daily_view(authed: APIClient) -> None:
    truncate_tables()

    resp = authed.do("POST", "/habits", {"name": "Test Weight", "description": "E2E test habit"})
    assert resp.status_code == 201, f"got status {resp.status_code}, want 201"
    habit_id = resp.json()["id"]

    log = {"habit_id": habit_id, "date": "2025-01-31", "value": 64.22}
    resp = authed.do("POST", "/habits/log", log)
    assert resp.status_code == 200, f"got status {resp.status_code}, want 200"

    habits = authed.do("GET", "/habits?date=2025-01-31").json()
    found = next((h for h in habits if h["id"] == habit_id), None)
    assert found is not None, f"habit with ID {habit_id} not found in daily view"
    assert found["log_value"] == 64.22
