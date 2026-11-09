from collections.abc import Sequence
from typing import Any, Literal

from fastapi import APIRouter, FastAPI
from fastapi.params import Depends

Tier = Literal["public", "semi", "full", "failover-token"]
TIER_TAG = "tier:"


def tier_router(tier: Tier, dependencies: Sequence[Depends] = ()) -> APIRouter:
    """A router whose routes all share one access tier; the tag makes the tier inspectable."""
    return APIRouter(tags=[TIER_TAG + tier], dependencies=list(dependencies))


def route_table(app: FastAPI) -> set[tuple[str, str, str]]:
    """(METHOD, path, tier) for every route, as tests/parity/routes.txt lists them."""
    table: set[tuple[str, str, str]] = set()
    paths: dict[str, dict[str, Any]] = app.openapi().get("paths", {})
    for path, operations in paths.items():
        for method, op in operations.items():
            tiers = [t.removeprefix(TIER_TAG) for t in op.get("tags", []) if t.startswith(TIER_TAG)]
            table.add((method.upper(), path, tiers[0] if len(tiers) == 1 else "?"))
    return table
