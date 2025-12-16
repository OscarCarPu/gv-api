"""make_frequency_non_nullable

Revision ID: 001_frequency
Revises: dbb8772805a1
Create Date: 2025-12-16

"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

# revision identifiers, used by Alembic.
revision: str = "001_frequency"
down_revision: str | Sequence[str] | None = "dbb8772805a1"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    """Make frequency column non-nullable with default 'daily'."""
    # First, update all NULL values to 'daily'
    op.execute("UPDATE habit SET frequency = 'daily' WHERE frequency IS NULL")

    # Then make the column non-nullable
    op.alter_column(
        "habit",
        "frequency",
        existing_type=sa.Enum("daily", "weekly", "monthly", name="targetfrequency"),
        nullable=False,
        server_default="daily",
    )


def downgrade() -> None:
    """Make frequency column nullable again."""
    op.alter_column(
        "habit",
        "frequency",
        existing_type=sa.Enum("daily", "weekly", "monthly", name="targetfrequency"),
        nullable=True,
        server_default=None,
    )
