import tempfile
from datetime import timedelta
from typing import Annotated, Any
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from pydantic import BeforeValidator, Field, ValidationError
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


def _origins(raw: Any) -> Any:
    if isinstance(raw, str):
        return [o.strip() for o in raw.split(",") if o.strip()]
    return raw


Ms = Annotated[int, Field(ge=0)]


class Config(BaseSettings):
    model_config = SettingsConfigDict(
        case_sensitive=False, extra="ignore", frozen=True, env_ignore_empty=True
    )

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
    printer_recording_max_minutes: int = 30 * 60
    printer_recordings_max_gb: float = 30.0
    printer_recording_video_codec: str = "copy"
    printer_recording_overlay: bool = True
    printer_recording_font: str = ""

    daily_capacity_hours: float = 14.0
    health_timeout_ms: Ms = 2000

    backup_dir: str = "/backups"
    backup_keep_hourly_days: int = 2
    backup_keep_daily_days: int = 30
    backup_interval_ms: Ms = 5 * 60 * 1000
    backup_s3_bucket: str = ""

    telegram_bot_token: str = ""
    telegram_chat_id: str = ""

    @property
    def failover_enabled(self) -> bool:
        """Home's side of the AWS failover runs: home with a token."""
        return self.failover_side == "home" and bool(self.failover_token)

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
    try:
        cfg = Config()
    except ValidationError as err:
        names = sorted({str(e["loc"][0]).upper() for e in err.errors() if e["loc"]})
        raise ConfigError("invalid env vars: " + ", ".join(names)) from None
    missing = [name for name in REQUIRED if not getattr(cfg, name.lower())]
    if missing:
        raise ConfigError("required env vars not set: " + ", ".join(missing))
    if cfg.backup_keep_hourly_days <= 0 or cfg.backup_keep_daily_days < cfg.backup_keep_hourly_days:
        raise ConfigError(
            "BACKUP_KEEP_HOURLY_DAYS must be at least 1 and at most BACKUP_KEEP_DAILY_DAYS"
        )
    if cfg.failover_side not in ("home", "aws"):
        raise ConfigError(f'invalid FAILOVER_SIDE "{cfg.failover_side}": must be home or aws')
    if cfg.failover_enabled and not cfg.backup_s3_bucket:
        raise ConfigError("FAILOVER_TOKEN is set but BACKUP_S3_BUCKET is not")
    try:
        cfg.location  # noqa: B018
    except (ZoneInfoNotFoundError, ValueError) as err:
        raise ConfigError(f'invalid TIMEZONE "{cfg.timezone}": {err}') from err
    return cfg
