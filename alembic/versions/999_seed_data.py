"""seed_data

Revision ID: 999
Revises: 002
Create Date: 2025-12-21 20:50:52.465548

"""

import random
from collections.abc import Sequence
from datetime import UTC, date, datetime, timedelta
from decimal import Decimal

import sqlalchemy as sa

from alembic import op

# revision identifiers, used by Alembic.
revision: str = "999"
down_revision: str | Sequence[str] | None = None
branch_labels: str | Sequence[str] | None = ("data_seed",)
depends_on: str | Sequence[str] | None = None

# Seed habits configuration (Spanish)
HABITS = [
    {
        "name": "Ejercicio",
        "description": "Actividad física semanal (correr, caminar, nadar, etc.)",
        "value_type": "numeric",
        "unit": "minutos",
        "frequency": "weekly",
        "target_value": 150,
        "target_min": None,
        "target_max": None,
        "comparison_type": "greater_equal_than",
        "default_value": None,
        "icon": "fas fa-dumbbell",
        "big_step": 30,
        "small_step": 10,
        "streak_strict": False,
    },
    {
        "name": "Lectura",
        "description": "Leer al menos 20 días al mes",
        "value_type": "boolean",
        "unit": None,
        "frequency": "monthly",
        "target_value": 20,
        "target_min": None,
        "target_max": None,
        "comparison_type": "greater_equal_than",
        "default_value": 1,
        "icon": "fas fa-book",
        "big_step": None,
        "small_step": None,
        "streak_strict": False,
    },
    {
        "name": "Agua",
        "description": "Mantenerse hidratado durante el día",
        "value_type": "numeric",
        "unit": "ml",
        "frequency": "daily",
        "target_value": 2000,
        "target_min": None,
        "target_max": None,
        "comparison_type": "greater_equal_than",
        "default_value": None,
        "icon": "fas fa-droplet",
        "big_step": 500,
        "small_step": 250,
        "streak_strict": False,
    },
    {
        "name": "Meditación",
        "description": "Práctica de meditación semanal",
        "value_type": "numeric",
        "unit": "minutos",
        "frequency": "weekly",
        "target_value": 60,
        "target_min": None,
        "target_max": None,
        "comparison_type": "greater_equal_than",
        "default_value": 1,
        "icon": "fas fa-brain",
        "big_step": 15,
        "small_step": 5,
        "streak_strict": False,
    },
    {
        "name": "Peso",
        "description": "Mantener peso entre 70-75kg",
        "value_type": "numeric",
        "unit": "kg",
        "frequency": "daily",
        "target_value": None,
        "target_min": 70,
        "target_max": 75,
        "comparison_type": "in_range",
        "default_value": None,
        "icon": "fas fa-weight-scale",
        "big_step": 1,
        "small_step": 0.1,
        "streak_strict": False,
    },
    {
        "name": "Comidas",
        "description": "Comer exactamente 3 comidas al día",
        "value_type": "numeric",
        "unit": "comidas",
        "frequency": "daily",
        "target_value": 3,
        "target_min": None,
        "target_max": None,
        "comparison_type": "equals",
        "default_value": 3,
        "icon": "fas fa-utensils",
        "big_step": 1,
        "small_step": 1,
        "streak_strict": False,
    },
    {
        "name": "Pasos",
        "description": "Caminar más de 10,000 pasos diarios",
        "value_type": "numeric",
        "unit": "pasos",
        "frequency": "daily",
        "target_value": 10000,
        "target_min": None,
        "target_max": None,
        "comparison_type": "greater_than",
        "default_value": None,
        "icon": "fas fa-shoe-prints",
        "big_step": 1000,
        "small_step": 500,
        "streak_strict": False,
    },
    {
        "name": "Pantalla",
        "description": "Limitar tiempo de pantalla a menos de 2 horas",
        "value_type": "numeric",
        "unit": "minutos",
        "frequency": "daily",
        "target_value": 120,
        "target_min": None,
        "target_max": None,
        "comparison_type": "less_than",
        "default_value": None,
        "icon": "fas fa-mobile-screen",
        "big_step": 30,
        "small_step": 15,
        "streak_strict": True,
    },
    {
        "name": "Cafeína",
        "description": "Mantener consumo de cafeína <= 400mg",
        "value_type": "numeric",
        "unit": "mg",
        "frequency": "daily",
        "target_value": 400,
        "target_min": None,
        "target_max": None,
        "comparison_type": "less_equal_than",
        "default_value": None,
        "icon": "fas fa-mug-hot",
        "big_step": 100,
        "small_step": 50,
        "streak_strict": False,
    },
]


def generate_value(habit_name: str, value_type: str) -> Decimal:
    """Generate realistic values based on habit type."""
    if value_type == "boolean":
        return Decimal("1") if random.random() < 0.8 else Decimal("0")
    elif habit_name == "Ejercicio":
        # Weekly target 150 min, log 30-60 min per session
        return Decimal(str(random.randint(30, 60)))
    elif habit_name == "Agua":
        return Decimal(str(random.randint(1000, 3000)))
    elif habit_name == "Meditación":
        return Decimal(str(random.randint(10, 30)))
    elif habit_name == "Peso":
        return Decimal(str(round(random.uniform(68, 77), 1)))
    elif habit_name == "Comidas":
        return Decimal(str(random.randint(2, 4)))
    elif habit_name == "Pasos":
        return Decimal(str(random.randint(5000, 15000)))
    elif habit_name == "Pantalla":
        return Decimal(str(random.randint(60, 180)))
    elif habit_name == "Cafeína":
        return Decimal(str(random.randint(100, 500)))
    return Decimal(str(random.randint(1, 100)))


def upgrade() -> None:
    """Insert seed data."""
    conn = op.get_bind()
    now = datetime.now(UTC)

    # Insert habits using raw SQL with proper enum casting
    for habit in HABITS:
        conn.execute(
            sa.text("""
                INSERT INTO habit (
                    name, description, value_type, unit, frequency,
                    target_value, target_min, target_max, comparison_type,
                    default_value, icon, big_step, small_step, streak_strict, created_at, updated_at
                ) VALUES (
                    :name, :description, CAST(:value_type AS valuetype), :unit,
                    CAST(:frequency AS targetfrequency), :target_value, :target_min,
                    :target_max, CAST(:comparison_type AS comparisontype),
                    :default_value, :icon, :big_step, :small_step, :streak_strict,
                    :created_at, :updated_at
                )
            """),
            {**habit, "created_at": now, "updated_at": now},
        )

    # Get inserted habits
    result = conn.execute(sa.text("SELECT id, name, value_type, default_value FROM habit"))
    habits = result.fetchall()

    # Generate logs for the past 60 days
    today = date.today()

    for habit_id, habit_name, value_type, default_value in habits:
        for day_offset in range(60):
            log_date = today - timedelta(days=day_offset)

            # Skip some days randomly
            skip_chance = 0.1 if default_value is None else 0.3
            if random.random() < skip_chance:
                continue

            value = generate_value(habit_name, value_type)
            conn.execute(
                sa.text("""
                    INSERT INTO habit_log (habit_id, log_date, value, created_at, updated_at)
                    VALUES (:habit_id, :log_date, :value, :created_at, :updated_at)
                """),
                {
                    "habit_id": habit_id,
                    "log_date": log_date,
                    "value": value,
                    "created_at": now,
                    "updated_at": now,
                },
            )


def downgrade() -> None:
    """Remove seed data."""
    op.execute("DELETE FROM habit_log")
    op.execute("DELETE FROM habit")
