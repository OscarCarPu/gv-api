import tempfile
from datetime import timedelta
from typing import Annotated, Any
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from pydantic import BeforeValidator, Field
from pydantic_core import PydanticUseDefault
from pydantic_settings import BaseSettings, NoDecode, SettingsConfigDict

REQUIRED = (
    "DATABASE_URL",
    "PASSWORD",
    "SEMIPRIVATE_PASSWORD",
    "JWT_SECRET",
    "TOTP_SECRET",
    "ALLOWED_ORIGINS",
)


class ConfigError(Exception):
    pass


# An empty or unparsable value falls back to the default instead of failing.
def _lenient(parse: Any) -> BeforeValidator:
    def validate(raw: Any) -> Any:
        if not isinstance(raw, str):
            return raw
        if raw == "":
            raise PydanticUseDefault
        try:
            return parse(raw)
        except ValueError:
            raise PydanticUseDefault from None

    return BeforeValidator(validate)


def _parse_ms(raw: str) -> int:
    ms = int(raw)
    if ms < 0:
        raise ValueError
    return ms


def _parse_bool(raw: str) -> bool:
    if raw in ("1", "t", "T", "TRUE", "true", "True"):
        return True
    if raw in ("0", "f", "F", "FALSE", "false", "False"):
        return False
    raise ValueError


def _origins(raw: Any) -> Any:
    if isinstance(raw, str):
        return [o.strip() for o in raw.split(",") if o.strip()]
    return raw


Ms = Annotated[int, _lenient(_parse_ms)]
Int = Annotated[int, _lenient(int)]
Float = Annotated[float, _lenient(float)]
Bool = Annotated[bool, _lenient(_parse_bool)]


class Config(BaseSettings):
    model_config = SettingsConfigDict(case_sensitive=False, extra="ignore", frozen=True)

    database_url: str = ""
    port: str = "8080"
    password: str = ""
    semiprivate_password: str = ""
    jwt_secret: str = ""
    totp_secret: str = ""
    timezone: str = "Europe/Madrid"
    allowed_origins: Annotated[list[str], NoDecode, BeforeValidator(_origins)] = Field(
        default_factory=list
    )
    failover_side: Annotated[str, BeforeValidator(lambda v: str(v).strip().lower())] = "home"
    failover_token: str = ""

    printer_rtsp_url: str = "rtsp://192.168.1.211/live"
    prusalink_host: Annotated[str, BeforeValidator(lambda v: str(v).rstrip("/"))] = ""
    prusalink_user: str = ""
    prusalink_password: str = ""
    prusalink_api_key: str = ""
    prusalink_storage: str = ""
    prusa_connect_user: str = ""
    prusa_connect_password: str = ""
    prusa_connect_printer_id: str = ""
    prusa_connect_token_file: str = "/data/prusa/connect.json"  # noqa: S105 (a path)
    printer_recordings_dir: str = "/data/recordings"
    printer_uploads_dir: str = Field(
        default_factory=lambda: tempfile.gettempdir() + "/gv-print-uploads"
    )
    printer_recording_max_minutes: Int = 30 * 60
    printer_recordings_max_gb: Float = 30.0
    printer_recording_video_codec: str = "copy"
    printer_recording_overlay: Bool = True
    printer_recording_font: str = ""

    daily_capacity_hours: Float = 14.0
    health_timeout_ms: Ms = 2000

    backup_dir: str = "/backups"
    backup_keep_hourly_days: Int = 2
    backup_keep_daily_days: Int = 30
    backup_interval_ms: Ms = 5 * 60 * 1000
    backup_s3_bucket: str = ""

    telegram_bot_token: str = ""
    telegram_chat_id: str = ""

    @property
    def on_failover(self) -> bool:
        return self.failover_side == "aws"

    @property
    def location(self) -> ZoneInfo:
        return ZoneInfo(self.timezone)

    @property
    def health_timeout(self) -> timedelta:
        return timedelta(milliseconds=self.health_timeout_ms)

    @property
    def backup_interval(self) -> timedelta:
        return timedelta(milliseconds=self.backup_interval_ms)

    @property
    def backup_keep_hourly(self) -> timedelta:
        return timedelta(days=self.backup_keep_hourly_days)

    @property
    def backup_keep_daily(self) -> timedelta:
        return timedelta(days=self.backup_keep_daily_days)

    @property
    def printer_recordings_max_bytes(self) -> int:
        return int(self.printer_recordings_max_gb * (1 << 30))


def load() -> Config:
    cfg = Config()
    missing = [name for name in REQUIRED if not getattr(cfg, name.lower())]
    if missing:
        raise ConfigError("required env vars not set: " + ", ".join(missing))
    if cfg.backup_keep_hourly_days <= 0 or cfg.backup_keep_daily_days < cfg.backup_keep_hourly_days:
        raise ConfigError(
            "BACKUP_KEEP_HOURLY_DAYS must be at least 1 and at most BACKUP_KEEP_DAILY_DAYS"
        )
    if cfg.failover_side not in ("home", "aws"):
        raise ConfigError(f'invalid FAILOVER_SIDE "{cfg.failover_side}": must be home or aws')
    try:
        cfg.location  # noqa: B018
    except (ZoneInfoNotFoundError, ValueError) as err:
        raise ConfigError(f'invalid TIMEZONE "{cfg.timezone}": {err}') from err
    return cfg
