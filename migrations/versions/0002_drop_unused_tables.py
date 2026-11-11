"""Drop tables nothing reads: lights and schema_migrations.

Revision ID: 0002_drop_unused_tables
Revises: 0001_baseline
Create Date: 2026-11-11
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0002_drop_unused_tables"
down_revision: str | None = "0001_baseline"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.execute("DROP TABLE IF EXISTS lights, schema_migrations")


def downgrade() -> None:
    raise NotImplementedError("dropped tables are not recreated")
