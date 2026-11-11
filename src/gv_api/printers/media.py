"""Signed links for the public printer routes (spec api-surface, signed URL compatibility).

A signature is the HMAC-SHA256 of `printer-media:<printer>/<name>/<exp>` keyed with JWT_SECRET.
Expiries are 12 h ahead rounded down to the hour, so a page polling the list gets stable links.
"""

import hashlib
import hmac
import time
from collections.abc import Callable
from urllib.parse import quote_plus

URL_TTL = 12 * 3600
URL_STEP = 3600


class Signer:
    def __init__(self, key: bytes, now: Callable[[], float] = time.time) -> None:
        self._key = key
        self._now = now

    def sign(self, printer_id: str, name: str, exp: int) -> str:
        message = f"printer-media:{printer_id}/{name}/{exp}".encode()
        return hmac.new(self._key, message, hashlib.sha256).hexdigest()

    def verify(self, printer_id: str, name: str, exp_raw: str, sig: str) -> bool:
        try:
            exp = int(exp_raw)
        except ValueError:
            return False
        if int(self._now()) > exp:
            return False
        return hmac.compare_digest(sig.encode(), self.sign(printer_id, name, exp).encode())

    def _expiry(self) -> int:
        at = int(self._now()) + URL_TTL
        return at - at % URL_STEP

    def media_url(self, printer_id: str, name: str) -> str:
        exp = self._expiry()
        sig = self.sign(printer_id, name, exp)
        return f"/domotics/printers/{printer_id}/recordings/{name}?exp={exp}&sig={sig}"

    def camera_url(self, printer_id: str) -> str:
        exp = self._expiry()
        sig = self.sign(printer_id, "camera", exp)
        return f"/domotics/printers/{printer_id}/camera/stream?exp={exp}&sig={sig}"

    def file_url(self, printer_id: str, name: str, display_name: str) -> str:
        exp = self._expiry()
        sig = self.sign(printer_id, "file/" + name, exp)
        return (
            f"/domotics/printers/{printer_id}/files/download?name={quote_plus(name)}"
            f"&as={quote_plus(display_name)}&exp={exp}&sig={sig}"
        )
