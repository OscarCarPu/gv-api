from dataclasses import dataclass, field
from pathlib import Path

from gv_api.config import Config


@dataclass(frozen=True)
class Printer:
    id: str
    name: str = ""
    model: str = ""
    rtsp: str = ""
    host: str = ""
    user: str = ""
    password: str = ""
    api_key: str = ""
    storage: str = ""

    connect_user: str = ""
    connect_password: str = ""
    connect_id: str = ""
    connect_base: str = ""
    connect_account: str = ""
    connect_token_file: str = ""


@dataclass(frozen=True)
class Settings:
    printers: list[Printer] = field(default_factory=list)
    recordings_dir: Path = Path("/data/recordings")
    uploads_dir: Path = Path("/tmp/gv-print-uploads")  # noqa: S108 (overridden from Config)
    max_minutes: int = 30 * 60
    max_bytes: int = 30 << 30
    video_codec: str = "copy"
    overlay: bool = True
    font: str = ""
    sign_key: bytes = b""
    ffmpeg: str = "ffmpeg"


def core_one(cfg: Config) -> Printer:
    return Printer(
        id="core-one",
        name="Prusa CORE One",
        model="Prusa CORE One + Buddy3D",
        rtsp=cfg.printer_rtsp_url,
        host=cfg.prusalink_host,
        user=cfg.prusalink_user,
        password=cfg.prusalink_password,
        api_key=cfg.prusalink_api_key,
        storage=cfg.prusalink_storage,
        connect_user=cfg.prusa_connect_user,
        connect_password=cfg.prusa_connect_password,
        connect_id=cfg.prusa_connect_printer_id,
        connect_token_file=cfg.prusa_connect_token_file,
    )


def settings(cfg: Config) -> Settings:
    return Settings(
        printers=[core_one(cfg)],
        recordings_dir=Path(cfg.printer_recordings_dir),
        uploads_dir=Path(cfg.printer_uploads_dir),
        max_minutes=cfg.printer_recording_max_minutes,
        max_bytes=cfg.printer_recordings_max_bytes,
        video_codec=cfg.printer_recording_video_codec,
        overlay=cfg.printer_recording_overlay,
        font=cfg.printer_recording_font,
        sign_key=cfg.jwt_secret.encode(),
    )
