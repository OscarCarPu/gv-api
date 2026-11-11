from sqlalchemy import (
    TIMESTAMP,
    CheckConstraint,
    Column,
    Date,
    Enum,
    ForeignKey,
    Integer,
    Numeric,
    Table,
    Text,
    UniqueConstraint,
    func,
    text,
)

from gv_api.db.metadata import metadata

transaction_type = Enum("income", "expense", "transfer", name="transaction_type", create_type=False)

accounts = Table(
    "accounts",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("name", Text, nullable=False),
    Column("total", Numeric(15, 2), nullable=False, server_default=text("0")),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    CheckConstraint("length(name) >= 1 AND length(name) <= 40"),
)

categories = Table(
    "categories",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("name", Text, nullable=False),
    Column("parent_id", Integer, ForeignKey("categories.id", ondelete="RESTRICT"), index=True),
    Column("type", transaction_type, nullable=False, index=True),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    CheckConstraint("parent_id IS NULL OR parent_id <> id"),
    CheckConstraint("length(name) >= 1 AND length(name) <= 40"),
)

transactions = Table(
    "transactions",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("type", transaction_type, nullable=False),
    Column("amount", Numeric(15, 2), nullable=False),
    Column("account_id", Integer, ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False),
    Column("to_account_id", Integer, ForeignKey("accounts.id", ondelete="RESTRICT")),
    Column("description", Text),
    Column("occurred_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("category_id", Integer, ForeignKey("categories.id", ondelete="RESTRICT")),
    CheckConstraint("amount > 0"),
)

budgets = Table(
    "budgets",
    metadata,
    Column("id", Integer, primary_key=True),
    Column(
        "category_id",
        Integer,
        ForeignKey("categories.id", ondelete="CASCADE"),
        nullable=False,
    ),
    Column("month", Date, nullable=False),
    Column("amount", Numeric(15, 2)),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("period", Text, nullable=False, server_default=text("'monthly'")),
    UniqueConstraint("category_id", "period", "month"),
)
