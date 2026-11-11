"""The Python route table against the Go release's (tests/parity/routes.txt, task 2.7)."""

from pathlib import Path

from gv_api.config import Config
from gv_api.core.routing import route_table
from gv_api.main import build

GO_ROUTES = Path(__file__).parents[1] / "parity" / "routes.txt"


def go_table() -> set[tuple[str, str, str]]:
    rows = set()
    for line in GO_ROUTES.read_text().splitlines():
        if line and not line.startswith("#"):
            method, path, tier = line.split()
            rows.add((method, path, tier))
    return rows


def python_table() -> set[tuple[str, str, str]]:
    cfg = Config(
        database_url="postgresql://u:p@127.0.0.1:1/db",
        password="p",
        semiprivate_password="s",
        jwt_secret="j",
        totp_secret="t",
        allowed_origins=["http://x"],
        failover_token="x",
        backup_s3_bucket="b",
    )
    return route_table(build(cfg))


def test_go_table_is_complete_and_has_no_lights() -> None:
    table = go_table()
    assert len(table) == 112
    assert not any(path.startswith("/domotics/lights") for _, path, _ in table)


def test_every_python_route_exists_in_go_with_the_same_tier() -> None:
    extra = python_table() - go_table()
    assert not extra, f"routes Go never served (or with another tier): {sorted(extra)}"


def test_python_serves_every_go_route() -> None:
    missing = go_table() - python_table()
    assert not missing, f"{len(missing)} Go routes not ported yet"
