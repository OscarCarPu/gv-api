"""Printer responses. Empty fields are left out (`router.send`): gv-web reads them as optional."""

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel


class Out(BaseModel):
    model_config = ConfigDict(
        alias_generator=to_camel, validate_by_name=True, serialize_by_alias=True
    )


class PublicPrinter(Out):
    id: str
    name: str
    model: str


class Temps(Out):
    nozzle: float | None = None
    nozzle_target: float | None = None
    bed: float | None = None
    bed_target: float | None = None
    chamber: float | None = None


class Fans(Out):
    hotend: float | None = None
    print: float | None = None


class JobInfo(Out):
    progress: float | None = None
    time_remaining: float | None = None
    time_printing: float | None = None
    file_name: str | None = None
    material: str | None = None


class Telemetry(Out):
    configured: bool
    online: bool = False
    state: str | None = None
    temps: Temps = Field(default_factory=Temps)
    fans: Fans = Field(default_factory=Fans)
    axis_z: float | None = None
    speed: float | None = None
    flow: float | None = None
    job: JobInfo | None = None
    error: str | None = None


class PrintMeta(Out):
    print_seconds: int | None = None
    material: str | None = None
    filament_grams: float | None = None
    filament_meters: float | None = None

    def has_meta(self) -> bool:
        return (
            self.print_seconds is not None
            or bool(self.material)
            or (self.filament_grams is not None or self.filament_meters is not None)
        )


class File(PrintMeta):
    name: str
    display_name: str
    size: int | None = None
    read_only: bool | None = None
    modified_at: int | None = None
    download_url: str | None = None


class Storage(Out):
    name: str
    available: bool
    read_only: bool
    free_space: int | None = None
    total_space: int | None = None


class FilesView(Out):
    online: bool
    storage: Storage | None = None
    files: list[File] = Field(default_factory=list)
    error: str | None = None


class FilesMeta(Out):
    files: dict[str, PrintMeta]


class UploadState(Out):
    sent: int = 0
    total: int = 0
    status: str
    error: str | None = None
    http_status: int | None = None


class ActiveUpload(UploadState):
    upload_id: str
    name: str
    from_: str | None = Field(default=None, alias="from")


class Uploads(Out):
    uploads: list[ActiveUpload]


class Recording(Out):
    name: str
    started_at: str
    ended_at: str | None = None
    duration_ms: int
    size_bytes: int
    recording: bool
    poster: str | None = None
    url: str | None = None
    poster_url: str | None = None


class RecordingsView(Out):
    recordings: list[Recording]
    used_bytes: int
    max_bytes: int


class PrintHistoryEntry(Out):
    state: str
    time_printing: float | None = None
    file_name: str
    material: str | None = None
    filament_grams: float | None = None
    filament_meters: float | None = None


class Chamber(Out):
    temp: float | None = None
    target_temp: float | None = None
