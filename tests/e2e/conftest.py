"""E2E harness: HTTP against a running API.

API_URL selects the API (`make test-e2e` rebuilds the compose API on the test database);
PASSWORD and TOTP_SECRET are that API's. TEST_DB_URL is its database, for the suites that
truncate tables.
"""

import asyncio
import os
from collections.abc import Iterator

import asyncpg
import httpx
import pyotp
import pytest


def pytest_collection_modifyitems(items: list[pytest.Item]) -> None:
    for item in items:
        if "tests/e2e/" in item.nodeid:
            item.add_marker(pytest.mark.e2e)


def api_url() -> str:
    url = os.environ.get("API_URL", "")
    if not url:
        pytest.skip("API_URL not set")
    return url


class APIClient:
    def __init__(self, base_url: str) -> None:
        self.http = httpx.Client(base_url=base_url, timeout=10)
        self.token = ""

    def do(self, method: str, path: str, json: object = None) -> httpx.Response:
        headers = {"Authorization": f"Bearer {self.token}"} if self.token else {}
        return self.http.request(method, path, json=json, headers=headers)

    def login(self, password: str) -> str:
        resp = self.do("POST", "/login", {"password": password})
        assert resp.status_code == 200, f"login: got status {resp.status_code}, want 200"
        return str(resp.json()["token"])

    def login_2fa(self, tmp_token: str, code: str) -> str:
        resp = self.do("POST", "/login/2fa", {"token": tmp_token, "code": code})
        assert resp.status_code == 200, f"2fa: got status {resp.status_code}, want 200"
        return str(resp.json()["token"])


def password() -> str:
    return os.environ.get("PASSWORD") or "Abc123.."


def totp_code() -> str:
    return pyotp.TOTP(os.environ.get("TOTP_SECRET") or "secret").now()


@pytest.fixture
def client() -> Iterator[APIClient]:
    c = APIClient(api_url())
    yield c
    c.http.close()


@pytest.fixture
def authed(client: APIClient) -> APIClient:
    client.token = client.login_2fa(client.login(password()), totp_code())
    return client


def truncate_tables() -> None:
    url = os.environ.get("TEST_DB_URL", "")
    if not url:
        pytest.skip("TEST_DB_URL not set")

    async def run() -> None:
        conn = await asyncpg.connect(url)
        try:
            await conn.execute(
                "TRUNCATE habits, habit_logs, projects, tasks, todos, time_entries,"
                " task_dependencies, calendar_events CASCADE"
            )
        finally:
            await conn.close()

    asyncio.run(run())
