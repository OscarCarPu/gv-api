"""Drop the habits feature: habits, habit_logs and recalculate_habit_streak.

Revision ID: 0003_drop_habits
Revises: 0002_drop_unused_tables
Create Date: 2026-11-17
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0003_drop_habits"
down_revision: str | None = "0002_drop_unused_tables"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.execute("DROP FUNCTION IF EXISTS recalculate_habit_streak(integer, date)")
    op.execute("DROP TABLE IF EXISTS habit_logs, habits")


def downgrade() -> None:
    raise NotImplementedError("dropped tables are not recreated")
