import argparse
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


def _not_ready(name: str) -> Callable[[argparse.Namespace], int]:
    def run(_: argparse.Namespace) -> int:
        print(f"{name}: not implemented yet", file=sys.stderr)
        return 2

    return run


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="gv-api", description="gv-api server and tools")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("serve", help="run the HTTP API").set_defaults(run=serve)
    sub.add_parser("backup", help="take one database backup and exit").set_defaults(
        run=_not_ready("backup")
    )
    sub.add_parser("prusa-login", help="log in to Prusa Connect interactively").set_defaults(
        run=_not_ready("prusa-login")
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    setup_logging()
    run: Callable[[argparse.Namespace], int] = args.run
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
