"""replace_is_required_with_default_value

Revision ID: 003
Revises: 002
Create Date: 2026-01-13 19:35:00.000000

"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

# revision identifiers, used by Alembic.
revision: str = "003"
down_revision: str | Sequence[str] | None = "002"
branch_labels: str | Sequence[str] | None = ("default",)
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    """Upgrade schema."""
    op.add_column("habit", sa.Column("default_value", sa.Numeric(10, 2), nullable=True))
    op.add_column(
        "habit", sa.Column("streak_strict", sa.Boolean(), nullable=False, server_default="0")
    )
    op.drop_column("habit", "is_required")


def downgrade() -> None:
    """Downgrade schema."""
    op.add_column(
        "habit", sa.Column("is_required", sa.Boolean(), nullable=False, server_default="true")
    )
    op.alter_column("habit", "is_required", server_default=None)
    op.drop_column("habit", "default_value")
    op.drop_column("habit", "streak_strict")
