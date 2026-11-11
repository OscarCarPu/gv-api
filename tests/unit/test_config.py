import os
from datetime import timedelta

import pytest

from gv_api.config import Config, ConfigError, load

REQUIRED_ENV = {
    "DATABASE_URL": "postgres://x",
    "PASSWORD": "p",
    "SEMIPRIVATE_PASSWORD": "s",
    "JWT_SECRET": "j",
    "TOTP_SECRET": "t",
    "ALLOWED_ORIGINS": "http://localhost",
}


@pytest.fixture(autouse=True)
def clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    for key in list(os.environ):
        if key.lower() in Config.model_fields:
            monkeypatch.delenv(key)
    for key, value in REQUIRED_ENV.items():
        monkeypatch.setenv(key, value)


def test_defaults() -> None:
    cfg = load()
    assert cfg.port == "8080"
    assert cfg.timezone == "Europe/Madrid"
    assert cfg.failover_side == "home"
    assert cfg.health_timeout == timedelta(seconds=2)
    assert cfg.backup_interval == timedelta(minutes=5)
    assert cfg.backup_keep_hourly == timedelta(days=2)
    assert cfg.backup_keep_daily == timedelta(days=30)
    assert cfg.daily_capacity_hours == 14.0
    assert cfg.printer_recording_overlay is True
    assert cfg.printer_recordings_max_bytes == 30 * (1 << 30)
    assert cfg.prusa_connect_token_file == "/data/prusa/connect.json"
    assert cfg.telegram_bot_token == ""


def test_missing_required_names_them_in_order(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("JWT_SECRET")
    monkeypatch.delenv("TOTP_SECRET")
    with pytest.raises(ConfigError) as err:
        load()
    assert str(err.value) == "required env vars not set: JWT_SECRET, TOTP_SECRET"


def test_empty_required_counts_as_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", "")
    monkeypatch.setenv("ALLOWED_ORIGINS", " , ")
    with pytest.raises(ConfigError) as err:
        load()
    assert str(err.value) == "required env vars not set: DATABASE_URL, ALLOWED_ORIGINS"


def test_allowed_origins_split_and_trimmed(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ALLOWED_ORIGINS", " http://a , ,http://b")
    assert load().allowed_origins == ["http://a", "http://b"]


@pytest.mark.parametrize(
    ("value", "want", "on_aws"),
    [(None, "home", False), ("home", "home", False), ("aws", "aws", True), ("AWS ", "aws", True)],
)
def test_failover_side(
    monkeypatch: pytest.MonkeyPatch, value: str | None, want: str, on_aws: bool
) -> None:
    if value is not None:
        monkeypatch.setenv("FAILOVER_SIDE", value)
    cfg = load()
    assert cfg.failover_side == want
    assert cfg.on_failover is on_aws


def test_failover_side_invalid(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("FAILOVER_SIDE", "foo")
    with pytest.raises(ConfigError, match='invalid FAILOVER_SIDE "foo": must be home or aws'):
        load()


def test_failover_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("FAILOVER_TOKEN", "")
    assert not load().failover_enabled
    monkeypatch.setenv("FAILOVER_TOKEN", "abc123")
    with pytest.raises(ConfigError, match="FAILOVER_TOKEN is set but BACKUP_S3_BUCKET is not"):
        load()
    monkeypatch.setenv("BACKUP_S3_BUCKET", "bucket")
    assert load().failover_enabled
    monkeypatch.setenv("FAILOVER_SIDE", "aws")
    assert not load().failover_enabled, "the backup server has no failover of its own"
    monkeypatch.delenv("BACKUP_S3_BUCKET")
    assert load().failover_token == "abc123"


def test_invalid_timezone(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TIMEZONE", "Mars/Olympus")
    with pytest.raises(ConfigError, match='invalid TIMEZONE "Mars/Olympus"'):
        load()


@pytest.mark.parametrize(("hourly", "daily"), [("0", "30"), ("5", "3")])
def test_backup_retention_bounds(monkeypatch: pytest.MonkeyPatch, hourly: str, daily: str) -> None:
    monkeypatch.setenv("BACKUP_KEEP_HOURLY_DAYS", hourly)
    monkeypatch.setenv("BACKUP_KEEP_DAILY_DAYS", daily)
    with pytest.raises(ConfigError, match="BACKUP_KEEP_HOURLY_DAYS must be at least 1"):
        load()


@pytest.mark.parametrize(
    ("key", "raw", "attr", "want"),
    [
        ("HEALTH_TIMEOUT_MS", "", "health_timeout_ms", 2000),
        ("HEALTH_TIMEOUT_MS", "750", "health_timeout_ms", 750),
        ("BACKUP_INTERVAL_MS", "0", "backup_interval_ms", 0),
        ("PRINTER_RECORDINGS_MAX_GB", "1.5", "printer_recordings_max_gb", 1.5),
        ("PRINTER_RECORDING_OVERLAY", "0", "printer_recording_overlay", False),
        ("PRINTER_RECORDING_OVERLAY", "yes", "printer_recording_overlay", True),
        ("PRINTER_RTSP_URL", "", "printer_rtsp_url", "rtsp://192.168.1.211/live"),
    ],
)
def test_typed_values_and_empty_is_unset(
    monkeypatch: pytest.MonkeyPatch, key: str, raw: str, attr: str, want: object
) -> None:
    monkeypatch.setenv(key, raw)
    assert getattr(load(), attr) == want


@pytest.mark.parametrize(
    ("key", "raw"),
    [
        ("HEALTH_TIMEOUT_MS", "abc"),
        ("HEALTH_TIMEOUT_MS", "-5"),
        ("DAILY_CAPACITY_HOURS", "nope"),
        ("PRINTER_RECORDING_OVERLAY", "maybe"),
    ],
)
def test_invalid_value_is_an_error(monkeypatch: pytest.MonkeyPatch, key: str, raw: str) -> None:
    monkeypatch.setenv(key, raw)
    with pytest.raises(ConfigError, match=f"invalid env vars: {key}"):
        load()


def test_prusalink_host_trailing_slash(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PRUSALINK_HOST", "http://printer//")
    assert load().prusalink_host == "http://printer"
