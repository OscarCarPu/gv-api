import logging
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.config import Config
from gv_api.db.engine import create_engine
from gv_api.db.migrate import migrate
from gv_api.failover.aws import S3Store, SSMParams
from gv_api.failover.controller import FailoverController
from gv_api.restore import Runner
from gv_api.uptime import UptimeRepository

log = logging.getLogger("gv_api")


def database_url(url: str, db: str) -> str:
    """The same server's connection string for another database."""
    parts = urlsplit(url)
    return urlunsplit(parts._replace(path="/" + db))


def controller(cfg: Config, engine: AsyncEngine) -> FailoverController:
    """Home's side of the failover. Talks to nothing until started."""
    home = urlsplit(cfg.database_url).path.lstrip("/")
    dir = Path(cfg.backup_dir) / "failover"

    async def release() -> None:
        # The app's connections go, so they reconnect to the database swapped into place.
        await engine.dispose()

    async def after() -> None:
        await engine.dispose()
        try:
            await migrate(engine)
        except Exception as err:
            log.error("failed to run migrations after the restore", extra={"error": str(err)})

    runner = Runner(
        admin=create_engine(database_url(cfg.database_url, "postgres"), pool_size=1),
        url=lambda db: database_url(cfg.database_url, db),
        home=home,
        dir=dir,
        # A trigger derives account totals from the transactions; outages is the heartbeat's.
        ignore={"accounts": ["total"]},
        skip=("outages",),
        last_alive=UptimeRepository.last_alive_of,
        release=release,
    )
    return FailoverController(
        cfg.failover_token, SSMParams(), S3Store(cfg.backup_s3_bucket), runner, dir, after
    )
