"""Budgets against actuals over the category tree.

Actuals roll up the tree within one transaction type. A subtree budgeted only in the other
period (monthly vs yearly) is left out of a budget's actuals, so no money counts twice.
"""

from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import date, datetime
from decimal import Decimal

from gv_api.finance.schemas import (
    ZERO,
    BudgetItem,
    BudgetTotals,
    Category,
    CategoryTotal,
    EffectiveBudget,
)

WARNING_SHARE = 0.8  # share of an expense budget from which it is a warning
AVERAGE_MONTHS = 3  # complete months behind the suggested monthly budget

Budgets = dict[int, EffectiveBudget]
Stop = Callable[[int], bool]


def period_start(period: str, d: date) -> date:
    return date(d.year, 1, 1) if period == "yearly" else date(d.year, d.month, 1)


def next_period(period: str, start: date) -> date:
    if period == "yearly":
        return date(start.year + 1, 1, 1)
    return date(start.year + start.month // 12, start.month % 12 + 1, 1)


def period_label(period: str, start: date) -> str:
    return start.strftime("%Y" if period == "yearly" else "%Y-%m")


def progress(now: datetime, start: datetime, end: datetime) -> float:
    """The elapsed share of [start, end) at now, within [0, 1]."""
    if now <= start:
        return 0.0
    if now >= end:
        return 1.0
    return (now - start) / (end - start)


def other_period_stop(own: Budgets, other: Budgets) -> Stop:
    """Stop at subtrees budgeted in the other period only."""
    return lambda id: id in other and id not in own


@dataclass
class Summary:
    items: list[BudgetItem] = field(default_factory=list)
    totals: dict[str, BudgetTotals] = field(
        default_factory=lambda: {"income": BudgetTotals(), "expense": BudgetTotals()}
    )


class CategoryTree:
    """The flat category list indexed by id and parent, in the list's order."""

    def __init__(self, cats: list[Category]) -> None:
        self.by_id = {c.id: c for c in cats}
        self.children: dict[int, list[int]] = {}
        self.roots: list[int] = []
        for c in cats:
            if c.parent_id is not None and c.parent_id in self.by_id:
                self.children.setdefault(c.parent_id, []).append(c.id)
            else:
                self.roots.append(c.id)

    def roll_up(self, totals: list[CategoryTotal], stop: Stop | None = None) -> dict[int, Decimal]:
        """Each category's total with its same-type descendants. Uncategorized totals are
        ignored."""
        direct: dict[int, Decimal] = {}
        for row in totals:
            if row.category_id is not None:
                direct[row.category_id] = direct.get(row.category_id, ZERO) + row.amount

        def total(id: int, type: str, seen: set[int]) -> Decimal:
            if id in seen:
                return ZERO
            seen.add(id)
            out = direct.get(id, ZERO) if self.by_id[id].type == type else ZERO
            for child in self.children.get(id, []):
                if stop is None or not stop(child):
                    out += total(child, type, seen)
            return out

        return {id: total(id, c.type, set()) for id, c in self.by_id.items()}

    def subtree(self, id: int, stop: Stop) -> list[int]:
        """The category and its same-type descendants, depth first."""
        type = self.by_id[id].type
        out: list[int] = []
        seen: set[int] = set()

        def walk(id: int) -> None:
            if id in seen:
                return
            seen.add(id)
            if self.by_id[id].type == type:
                out.append(id)
            for child in self.children.get(id, []):
                if not stop(child):
                    walk(child)

        walk(id)
        return out

    def covered(self, *sets: Budgets) -> set[int]:
        """Categories budgeted, or under a same-type budgeted ancestor, in any of the sets."""
        out: set[int] = set()
        done: set[int] = set()

        def walk(id: int, inherited: frozenset[str]) -> None:
            if id in done:
                return
            done.add(id)
            c = self.by_id[id]
            here = c.type in inherited or any(id in s for s in sets)
            if here:
                out.add(id)
                inherited = inherited | {c.type}
            for child in self.children.get(id, []):
                walk(child, inherited)

        for root in self.roots:
            walk(root, frozenset())
        return out

    def summarize(
        self, period: str, own: Budgets, other: Budgets, totals: list[CategoryTotal]
    ) -> Summary:
        """One period's budget rows, nested depth first in category order, and its totals."""
        actual = self.roll_up(totals, other_period_stop(own, other))
        out = Summary()

        def walk(id: int, depth: dict[str, int]) -> Decimal:
            # Returns the subtree's overspend: a euro over several nested budgets counts once.
            c = self.by_id[id]
            b = own.get(id)
            if b is not None:
                d = depth.get(c.type, 0)
                out.items.append(budget_item(period, c, b, actual[id], d))
                if d == 0 and c.type in out.totals:
                    t = out.totals[c.type]
                    t.budgeted += b.amount
                    t.actual += actual[id]
                depth = depth | {c.type: d + 1}
            over = sum((walk(child, depth) for child in self.children.get(id, [])), ZERO)
            if b is not None and c.type == "expense":
                over = max(over, actual[id] - b.amount)
            return over

        overspent = sum((walk(root, {}) for root in self.roots), ZERO)
        out.totals["expense"].overspent = overspent
        return out


def budget_item(
    period: str, c: Category, b: EffectiveBudget, actual: Decimal, depth: int
) -> BudgetItem:
    if b.amount > 0:
        share = float(actual / b.amount)
    elif actual > 0:
        share = 1.0
    else:
        share = 0.0
    if c.type == "income":
        status = "met" if actual >= b.amount else "pending"
    elif actual > b.amount:
        status = "over"
    elif share >= WARNING_SHARE:
        status = "warning"
    else:
        status = "ok"
    return BudgetItem(
        category_id=c.id,
        period=period,
        name=c.name,
        parent_id=c.parent_id,
        type=c.type,
        depth=depth,
        budget=b.amount,
        actual=actual,
        remaining=b.amount - actual,
        progress=share,
        status=status,
        since=period_label(period, b.since),
    )
