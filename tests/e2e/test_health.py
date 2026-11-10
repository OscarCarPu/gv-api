from tests.e2e.conftest import APIClient


def test_health(client: APIClient) -> None:
    assert client.do("GET", "/health").status_code == 200
