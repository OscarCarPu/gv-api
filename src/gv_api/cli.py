import argparse
import asyncio
import logging
import sys
from collections.abc import Callable, Sequence

from gv_api import alerts
from gv_api.config import Config, ConfigError, load
from gv_api.core.logging import setup_logging

log = logging.getLogger("gv_api")


def _config() -> Config | None:
    try:
        return load()
    except ConfigError as err:
        log.error("failed to load config", extra={"error": str(err)})
        return None


def serve(_: argparse.Namespace) -> int:
    from gv_api.main import build, run_server

    cfg = _config()
    if cfg is None:
        return 1
    handler = alerts.install(cfg)
    try:
        return run_server(build(cfg, handler), cfg)
    finally:
        if handler is not None:
            handler.flush_on_exit()
            alerts.uninstall(handler)


BACKUP_TIMEOUT = 600


def backup(_: argparse.Namespace) -> int:
    cfg = _config()
    if cfg is None:
        return 1
    handler = alerts.install(cfg)
    try:
        return asyncio.run(_backup(cfg))
    finally:
        if handler is not None:
            handler.flush_on_exit()
            alerts.uninstall(handler)


async def _backup(cfg: Config) -> int:
    from gv_api.backup import backup_service
    from gv_api.db.engine import create_engine, ping

    engine = create_engine(cfg.database_url, pool_size=2)
    try:
        try:
            await ping(engine)
        except Exception as err:
            log.error("failed to connect to database", extra={"error": str(err)})
            return 1
        try:
            svc = backup_service(cfg, engine)
        except Exception as err:
            log.error("failed to set up backups", extra={"error": str(err)})
            return 1
        try:
            done = await asyncio.wait_for(svc.run(), BACKUP_TIMEOUT)
        except Exception as err:
            log.error("backup failed", extra={"error": str(err) or type(err).__name__})
            return 1
        log.info("backup done", extra={"file": done.name, "size": done.size})
        return 0
    finally:
        await engine.dispose()


PRUSA_LOGIN_TIMEOUT = 300


def prusa_login(_: argparse.Namespace) -> int:
    from gv_api.printers import core_one
    from gv_api.printers.connect import ConnectClient

    cfg = _config()
    if cfg is None:
        return 1

    async def ask_code() -> str:
        print("Prusa sent a code to your email. Code: ", end="", flush=True)
        line: str = await asyncio.to_thread(sys.stdin.readline)
        if not line:
            raise EOFError("no code entered")
        return line.strip()

    async def login() -> None:
        client = ConnectClient(core_one(cfg))
        try:
            await asyncio.wait_for(client.save_login(ask_code), PRUSA_LOGIN_TIMEOUT)
        finally:
            await client.aclose()

    try:
        asyncio.run(login())
    except Exception as err:
        print("Login failed:", str(err) or type(err).__name__, file=sys.stderr)
        return 1
    print("Logged in. The running API will pick the session up on its next request.")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="gv-api", description="gv-api server and tools")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("serve", help="run the HTTP API").set_defaults(run=serve)
    sub.add_parser("backup", help="take one database backup and exit").set_defaults(run=backup)
    sub.add_parser("prusa-login", help="log in to Prusa Connect interactively").set_defaults(
        run=prusa_login
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    setup_logging()
    run: Callable[[argparse.Namespace], int] = args.run
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
