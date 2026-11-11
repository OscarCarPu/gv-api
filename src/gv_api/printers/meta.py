"""Print estimate and filament use from gcode metadata, as PrusaLink and Prusa Connect report it."""

import math
import re
from typing import Any

from gv_api.printers.schemas import PrintMeta

TIME_KEYS = (
    "estimated_print_time",
    "estimated printing time (normal mode)",
    "estimated_printing_time_normal_mode",
    "print_time",
    "printing_time",
)
GRAM_KEYS = ("filament used [g]", "filament_used_g", "filament_used [g]")
MM_KEYS = ("filament used [mm]", "filament_used_mm", "filament_used [mm]")

_DURATION = re.compile(r"(\d+)\s*([dhms])")
_UNIT_SECONDS = {"d": 86400, "h": 3600, "m": 60, "s": 1}
_SEPARATOR = re.compile(r"[,;]")


def round_to(x: float, places: int) -> float:
    """Half up, as people round a reading (12.25 g -> 12.3)."""
    scale = 10.0**places
    return math.floor(x * scale + 0.5) / scale


def _number(v: Any) -> float | None:
    return float(v) if isinstance(v, int | float) and not isinstance(v, bool) else None


def duration_seconds(v: Any) -> int | None:
    """Seconds from a number or a text such as "1d 2h 3m"; None unless positive."""
    n = _number(v)
    if isinstance(v, str):
        try:
            n = float(v.strip())
        except ValueError:
            total = sum(int(m[1]) * _UNIT_SECONDS[m[2]] for m in _DURATION.finditer(v))
            return total if total > 0 else None
    if n is None or not math.isfinite(n) or n <= 0:
        return None
    return int(n)


def sum_numbers(v: Any) -> float | None:
    """A number, or the sum of a "10.04, 2.5" list (one per extruder); None unless positive."""
    n = _number(v)
    if isinstance(v, str):
        parts = []
        for part in _SEPARATOR.split(v):
            try:
                parts.append(float(part.strip()))
            except ValueError:
                continue
        n = sum(parts) if parts else None
    return n if n is not None and n > 0 else None


def material_list(raw: str) -> str:
    seen = dict.fromkeys(p.strip() for p in _SEPARATOR.split(raw))
    return ", ".join(p for p in seen if p)


def parse_print_meta(meta: dict[str, Any]) -> PrintMeta:
    out = PrintMeta()
    seconds = (duration_seconds(meta[k]) for k in TIME_KEYS if k in meta)
    out.print_seconds = next((s for s in seconds if s is not None), None)
    if isinstance(t := meta.get("filament_type"), str):
        out.material = material_list(t) or None
    if (g := _first_sum(meta, GRAM_KEYS)) is not None:
        out.filament_grams = round_to(g, 1)
    if (mm := _first_sum(meta, MM_KEYS)) is not None:
        out.filament_meters = round_to(mm / 1000, 2)
    elif (m := sum_numbers(meta.get("filament_used_m"))) is not None:
        out.filament_meters = round_to(m, 2)
    return out


def _first_sum(meta: dict[str, Any], keys: tuple[str, ...]) -> float | None:
    sums = (sum_numbers(meta.get(k)) for k in keys)
    return next((s for s in sums if s is not None), None)


def done_fraction(
    state: str, progress: float | None, time_printing: float | None, estimate: int | None
) -> float:
    """The share of a stopped or failed print that ran, to scale its filament use."""
    if state not in ("FIN_STOPPED", "FIN_ERROR"):
        return 1
    if progress is not None and progress > 0:
        return min(1, progress / 100)
    if time_printing is not None and estimate:
        return min(1, time_printing / estimate)
    return 1


def scale_used(v: float | None, done: float) -> float | None:
    if v is None or done >= 1:
        return v
    return round_to(v * done, 1)
