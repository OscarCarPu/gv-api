"""ERROR logs to a Telegram chat, batched every 10 s (design D12)."""

import asyncio
import logging
import threading
from dataclasses import dataclass

import httpx

from gv_api.config import Config
from gv_api.core.logging import RequestIDFilter

API_URL = "https://api.telegram.org"
FLUSH_INTERVAL = 10.0
SEND_TIMEOUT = 5.0
EXIT_FLUSH_TIMEOUT = 5.0
MESSAGE_LIMIT = 4096
MAX_ENTRIES = 100

# Delivery problems go here; the handler ignores this logger so it never alerts about itself.
log = logging.getLogger("gv_api.alerts")


@dataclass
class _Entry:
    msg: str
    error: str | None
    request_id: str | None
    count: int = 1


def _block(entry: _Entry) -> str:
    lines = [entry.msg if entry.count == 1 else f"{entry.msg} (x{entry.count})"]
    if entry.error:
        lines.append(f"error: {entry.error}")
    if entry.request_id:
        lines.append(f"request_id: {entry.request_id}")
    return "\n".join(lines)


def _not_shown(n: int) -> str:
    return f"\n\n[truncated: {n} more not shown]" if n else ""


def render(side: str, entries: list[_Entry], dropped: int = 0) -> str:
    """One message of at most MESSAGE_LIMIT characters that says so when it is cut."""
    total = sum(e.count for e in entries) + dropped
    head = f"gv-api {side}: {total} error{'' if total == 1 else 's'}"
    footer = f"\n\n[+{dropped} more dropped: buffer full]" if dropped else ""
    blocks = ["\n\n" + _block(e) for e in entries]
    text = head + "".join(blocks) + footer
    if len(text) <= MESSAGE_LIMIT:
        return text

    shown, n = head, 0
    for block in blocks:
        tail = _not_shown(len(blocks) - n - 1) + footer
        if len(shown) + len(block) + len(tail) > MESSAGE_LIMIT:
            break
        shown += block
        n += 1
    if n == 0:  # a single entry longer than the limit
        tail = "\n[truncated]" + _not_shown(len(blocks) - 1) + footer
        return shown + blocks[0][: MESSAGE_LIMIT - len(shown) - len(tail)] + tail
    return shown + _not_shown(len(blocks) - n) + footer


class TelegramHandler(logging.Handler):
    """`emit` only updates an in-memory batch; `flush_batch` sends it."""

    def __init__(
        self,
        token: str,
        chat_id: str,
        side: str,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        super().__init__(level=logging.ERROR)
        self._transport = transport
        self._url = f"{API_URL}/bot{token}/sendMessage"
        self._token = token
        self._chat_id = chat_id
        self._side = side
        self._entries: dict[tuple[str, str | None], _Entry] = {}
        self._dropped = 0
        self._mu = threading.Lock()
        self.addFilter(RequestIDFilter())
        self.addFilter(lambda r: not r.name.startswith(log.name))

    def emit(self, record: logging.LogRecord) -> None:
        try:
            msg = record.getMessage()
            error = record.__dict__.get("error")
            if error is None and record.exc_info and record.exc_info[1] is not None:
                error = record.exc_info[1]
            error = None if error is None else str(error)
            key = (msg, error)
            with self._mu:
                if entry := self._entries.get(key):
                    entry.count += 1
                elif len(self._entries) < MAX_ENTRIES:
                    rid = record.__dict__.get("request_id")
                    self._entries[key] = _Entry(msg, error, None if rid is None else str(rid))
                else:
                    self._dropped += 1
        except Exception:
            self.handleError(record)

    def _take(self) -> tuple[list[_Entry], int]:
        with self._mu:
            entries, dropped = list(self._entries.values()), self._dropped
            self._entries, self._dropped = {}, 0
        return entries, dropped

    def _redact(self, text: str) -> str:
        return text.replace(self._token, "***")

    async def flush_batch(self) -> None:
        """Send what is pending. Never raises: a failed batch is dropped with a WARNING."""
        entries, dropped = self._take()
        if not entries and not dropped:
            return
        records = sum(e.count for e in entries) + dropped
        payload = {
            "chat_id": self._chat_id,
            "text": render(self._side, entries, dropped),
            "disable_web_page_preview": True,
        }
        try:
            async with httpx.AsyncClient(timeout=SEND_TIMEOUT, transport=self._transport) as client:
                resp = await client.post(self._url, json=payload)
            if resp.status_code != httpx.codes.OK:
                log.warning(
                    "telegram: alert batch dropped",
                    extra={
                        "records": records,
                        "status": resp.status_code,
                        "error": self._redact(resp.text[:200]),
                    },
                )
        except Exception as err:
            log.warning(
                "telegram: alert batch dropped",
                extra={"records": records, "error": self._redact(f"{type(err).__name__}: {err}")},
            )

    async def run(self) -> None:
        while True:
            await asyncio.sleep(FLUSH_INTERVAL)
            await self.flush_batch()

    def flush_on_exit(self, timeout: float = EXIT_FLUSH_TIMEOUT) -> None:
        """Blocking final send, for the CLI after the event loop is gone."""
        try:
            asyncio.run(asyncio.wait_for(self.flush_batch(), timeout))
        except TimeoutError:
            log.warning("telegram: alert batch dropped", extra={"error": "exit flush timed out"})


def install(cfg: Config) -> TelegramHandler | None:
    """Attach the handler to the root logger, or log once that alerts are off."""
    if not cfg.telegram_bot_token or not cfg.telegram_chat_id:
        log.info("telegram alerts disabled: TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set")
        return None
    origin = f"{cfg.environment}/{cfg.failover_side}"
    handler = TelegramHandler(cfg.telegram_bot_token, cfg.telegram_chat_id, origin)
    logging.getLogger().addHandler(handler)
    return handler


def uninstall(handler: TelegramHandler) -> None:
    logging.getLogger().removeHandler(handler)
