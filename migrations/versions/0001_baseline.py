"""Baseline: the initial schema (migrations/baseline.sql).

The production database already has this schema; it is stamped at this revision instead of
running it.

Revision ID: 0001_baseline
Revises:
Create Date: 2026-11-09
"""

from collections.abc import Sequence
from pathlib import Path

from alembic import op
from sqlalchemy.util import await_only

revision: str = "0001_baseline"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

BASELINE = Path(__file__).resolve().parents[1] / "baseline.sql"


def upgrade() -> None:
    # A multi-statement script: asyncpg's prepared statements take one statement, so it runs
    # on the driver connection, still inside Alembic's transaction.
    driver = op.get_bind().connection.driver_connection
    await_only(driver.execute(BASELINE.read_text()))
    # Uptime counts from this marker.
    op.execute("INSERT INTO outages (ends_at, kind) VALUES (now(), 'start')")


def downgrade() -> None:
    raise NotImplementedError("the baseline cannot be downgraded")
