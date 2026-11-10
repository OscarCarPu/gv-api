"""PlanService over a recording fake repository."""

from datetime import UTC, date, datetime, time, timedelta
from decimal import Decimal
from typing import Any, cast
from zoneinfo import ZoneInfo

import pytest

from gv_api.plan import PlanService
from gv_api.plan.errors import (
    DaysOfWeekRequiredError,
    InvalidTimeRangeError,
    LabelRequiredError,
    LabelTooLongError,
    NotFoundError,
    OverlapError,
    TaskNotFoundError,
)
from gv_api.plan.schemas import (
    CreateCommitmentRequest,
    CreatePlanBlockRequest,
    NewBlock,
    PlanBlockResponse,
    RecurringCommitmentResponse,
    UpdateCommitmentRequest,
    UpdatePlanBlockRequest,
)
from gv_api.plan.service import Repository, parse_hhmm, weekday
from gv_api.tasks.schemas import TimeEntrySummaryResponse

from .test_tasks_service import FakeRepo

MADRID = ZoneInfo("Europe/Madrid")
UTC_ZONE = ZoneInfo("UTC")
NOW = datetime(2026, 5, 10, 8, tzinfo=UTC)
SUMMARY = TimeEntrySummaryResponse(
    today=7200, week=100000, daily_target_seconds=30000, weekly_target_seconds=1
)


class Summary:
    def __init__(self, err: Exception | None = None) -> None:
        self.err = err

    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse:
        if self.err:
            raise self.err
        return SUMMARY


def svc(repo: FakeRepo, loc: ZoneInfo = UTC_ZONE, now: datetime = NOW) -> PlanService:
    return PlanService(cast(Repository, repo), Summary(), loc, lambda: now)


def t(hour: float, day: int = 10) -> datetime:
    return datetime(2026, 5, day, tzinfo=UTC) + timedelta(hours=hour)


def block(id: int, start: datetime, end: datetime, **kw: Any) -> PlanBlockResponse:
    return PlanBlockResponse.model_construct(
        id=id, plan_date=start.date(), started_at=start, ended_at=end, label="x", **kw
    )


def commitment(**kw: Any) -> RecurringCommitmentResponse:
    data: dict[str, Any] = {
        "id": 7,
        "task_id": 42,
        "task_name": "Work",
        "label": "Work",
        "days_of_week": [1],
        "start_time": "09:00",
        "end_time": "13:00",
        "active": True,
    }
    return RecurringCommitmentResponse.model_construct(**(data | kw))


def create_req(**data: Any) -> CreatePlanBlockRequest:
    return CreatePlanBlockRequest.model_validate({"started_at": t(11), "ended_at": t(12)} | data)


# Create


@pytest.mark.parametrize(
    ("data", "err"),
    [
        ({"ended_at": t(11), "label": "foo"}, InvalidTimeRangeError),
        ({"started_at": None, "label": "foo"}, InvalidTimeRangeError),
        ({}, LabelRequiredError),
        ({"label": "  "}, LabelRequiredError),
        ({"label": "a" * 201}, LabelTooLongError),
    ],
)
async def test_create_rejects(data: dict[str, Any], err: type[Exception]) -> None:
    repo = FakeRepo()
    with pytest.raises(err):
        await svc(repo).create(create_req(**data))
    assert repo.calls == []


async def test_create_free_block_trims_label_and_dates_it_locally() -> None:
    repo = FakeRepo(has_overlap=False, create="created")
    start = datetime(2026, 5, 10, 22, 30, tzinfo=UTC)  # 00:30 on the 11th in Madrid
    req = create_req(started_at=start, ended_at=start + timedelta(hours=1), label="  comer  ")
    assert await svc(repo, MADRID).create(req) == "created"
    assert repo.calls[0] == ("has_overlap", start, start + timedelta(hours=1), None)
    new: NewBlock = repo.calls[1][1]
    assert (new.label, new.task_id, new.note, new.plan_date) == (
        "comer",
        None,
        None,
        date(2026, 5, 11),
    )


async def test_create_rejects_overlap() -> None:
    repo = FakeRepo(has_overlap=True)
    with pytest.raises(OverlapError):
        await svc(repo).create(create_req(label="comer"))
    assert repo.names() == ["has_overlap"]


async def test_create_linked_block_falls_back_to_the_task_name() -> None:
    repo = FakeRepo(get_task_name="Refactor agenda", has_overlap=False)
    await svc(repo).create(create_req(task_id=42))
    assert repo.calls[0] == ("get_task_name", 42)
    assert repo.calls[2][1].label == "Refactor agenda"


async def test_create_linked_block_keeps_its_own_label() -> None:
    repo = FakeRepo(has_overlap=False)
    await svc(repo).create(create_req(task_id=42, label="Sprint planning"))
    assert repo.names() == ["has_overlap", "create"]
    assert (repo.calls[1][1].task_id, repo.calls[1][1].label) == (42, "Sprint planning")


async def test_create_unknown_task() -> None:
    repo = FakeRepo(get_task_name=TaskNotFoundError())
    with pytest.raises(TaskNotFoundError):
        await svc(repo).create(create_req(task_id=999))


# Update


async def test_update_rejects_inverted_times() -> None:
    repo = FakeRepo()
    with pytest.raises(InvalidTimeRangeError):
        await svc(repo).update(1, UpdatePlanBlockRequest(started_at=t(11), ended_at=t(10)))
    assert repo.calls == []


async def test_update_checks_one_bound_against_the_stored_other() -> None:
    repo = FakeRepo(get=block(1, t(11), t(12)))
    with pytest.raises(InvalidTimeRangeError):
        await svc(repo).update(1, UpdatePlanBlockRequest(started_at=t(13)))
    assert repo.names() == ["get"]


async def test_update_rejects_an_empty_or_long_label() -> None:
    with pytest.raises(LabelRequiredError):
        await svc(FakeRepo()).update(1, UpdatePlanBlockRequest(label="   "))
    with pytest.raises(LabelTooLongError):
        await svc(FakeRepo()).update(1, UpdatePlanBlockRequest(label="a" * 201))


async def test_update_rejects_overlap_excluding_itself() -> None:
    repo = FakeRepo(has_overlap=True)
    with pytest.raises(OverlapError):
        await svc(repo).update(7, UpdatePlanBlockRequest(started_at=t(11), ended_at=t(12)))
    assert repo.calls == [("has_overlap", t(11), t(12), 7)]


async def test_update_label_task_and_note_only() -> None:
    repo = FakeRepo(update="updated")
    req = UpdatePlanBlockRequest(label=" new ", task_id=3, note="n")
    assert await svc(repo).update(5, req) == "updated"
    assert repo.calls == [("update", 5, {"label": "new", "task_id": 3, "note": "n"})]


async def test_update_clears_task_and_note() -> None:
    repo = FakeRepo()
    req = UpdatePlanBlockRequest(task_id=3, clear_task=True, note="n", clear_note=True)
    await svc(repo).update(5, req)
    assert repo.calls == [("update", 5, {"task_id": None, "note": None})]


async def test_update_end_only_keeps_the_plan_date() -> None:
    repo = FakeRepo(get=block(5, t(11), t(12)), has_overlap=False)
    await svc(repo).update(5, UpdatePlanBlockRequest(ended_at=t(13)))
    assert repo.names() == ["get", "has_overlap", "update"]
    assert repo.calls[-1] == ("update", 5, {"ended_at": t(13)})


async def test_update_start_sets_the_utc_plan_date() -> None:
    repo = FakeRepo(get=block(5, t(20), t(25)), has_overlap=False)
    start = datetime(2026, 5, 10, 22, 30, tzinfo=UTC)
    await svc(repo, MADRID).update(5, UpdatePlanBlockRequest(started_at=start))
    assert repo.calls[-1][2]["plan_date"] == date(2026, 5, 10)


async def test_update_moving_a_generated_block_to_another_day_detaches_it() -> None:
    current = block(5, t(9), t(10), commitment_id=7)
    repo = FakeRepo(get=current, has_overlap=False)
    await svc(repo).update(5, UpdatePlanBlockRequest(started_at=t(9, 11), ended_at=t(10, 11)))
    assert repo.calls[2] == ("insert_skip", 7, date(2026, 5, 10))
    assert repo.calls[3][2]["commitment_id"] is None


async def test_update_within_the_same_day_keeps_the_commitment() -> None:
    repo = FakeRepo(get=block(5, t(9), t(10), commitment_id=7), has_overlap=False)
    await svc(repo).update(5, UpdatePlanBlockRequest(started_at=t(15), ended_at=t(16)))
    assert repo.names() == ["has_overlap", "get", "update"]
    assert "commitment_id" not in repo.calls[-1][2]


# Delete and event links


async def test_delete_skips_the_date_of_a_generated_block() -> None:
    repo = FakeRepo(get=block(5, t(9), t(10), commitment_id=7))
    await svc(repo).delete(5)
    assert repo.calls[1:] == [("insert_skip", 7, date(2026, 5, 10)), ("delete", 5)]


async def test_delete_plain_block_and_missing_block() -> None:
    repo = FakeRepo(get=block(5, t(9), t(10)))
    await svc(repo).delete(5)
    assert repo.names() == ["get", "delete"]
    with pytest.raises(NotFoundError):
        await svc(FakeRepo(get=NotFoundError())).delete(5)


async def test_delete_future_uses_the_clock() -> None:
    repo = FakeRepo()
    await svc(repo).delete_future()
    assert repo.calls == [("delete_ending_after", NOW)]


async def test_event_sync_and_detach() -> None:
    repo = FakeRepo(get_by_event_ref=block(5, t(9), t(10)))
    await svc(repo).sync_event_time("ev", t(11), t(12))
    await svc(repo).detach_event("ev")
    assert repo.calls[1] == ("update_times", 5, t(11), t(12))
    assert repo.calls[3] == ("clear_event_ref", 5)

    missing = FakeRepo(get_by_event_ref=NotFoundError())
    await svc(missing).sync_event_time("ev", t(11), t(12))
    await svc(missing).detach_event("ev")
    assert missing.names() == ["get_by_event_ref", "get_by_event_ref"]


# Today and range


async def test_today_totals_and_budget() -> None:
    repo = FakeRepo(
        list_by_date=[
            block(1, t(11), t(12), task_id=42),
            block(2, t(13), t(15)),
            block(3, t(16), t(16.5), task_id=43),
        ]
    )
    resp = await svc(repo).get_today()
    assert repo.calls == [("list_by_date", date(2026, 5, 10))]
    assert (resp.totals.task_seconds, resp.totals.free_seconds) == (5400, 7200)
    assert resp.budget == SUMMARY
    assert resp.model_dump(mode="json")["date"] == "2026-05-10"


async def test_today_counts_only_free_time_still_ahead() -> None:
    repo = FakeRepo(
        list_by_date=[block(1, t(9), t(11)), block(2, t(13), t(15)), block(3, t(16), t(17))]
    )
    resp = await svc(repo, now=t(14)).get_today()
    assert resp.totals.free_seconds == 2 * 3600


async def test_today_uses_the_local_day() -> None:
    repo = FakeRepo(list_by_date=[])
    await svc(repo, MADRID, now=t(23)).get_today()
    assert repo.calls == [("list_by_date", date(2026, 5, 11))]


async def test_today_propagates_a_budget_error() -> None:
    boom = RuntimeError("boom")
    service = PlanService(cast(Repository, FakeRepo(list_by_date=[])), Summary(boom), MADRID)
    with pytest.raises(RuntimeError):
        await service.get_today()


async def test_range_generates_then_lists_utc_midnights() -> None:
    repo = FakeRepo(list_active_commitments=[], list_by_range=[])
    resp = await svc(repo, MADRID).get_range(date(2026, 5, 4), date(2026, 5, 11))
    assert repo.calls == [
        ("list_active_commitments",),
        ("list_by_range", datetime(2026, 5, 4, tzinfo=UTC), datetime(2026, 5, 11, tzinfo=UTC)),
    ]
    assert resp.model_dump(mode="json") == {"from": "2026-05-04", "to": "2026-05-11", "blocks": []}


# Generation


async def test_generation_fills_matching_weekdays_not_existing_or_skipped() -> None:
    monday = date(2026, 5, 4)
    repo = FakeRepo(
        list_active_commitments=[commitment(days_of_week=[1, 3, 5, 0])],
        list_taken_dates={
            (7, monday),
            (7, monday + timedelta(days=2)),
            (8, monday + timedelta(days=4)),
        },
    )
    await svc(repo, MADRID).ensure_recurring_blocks(monday, monday + timedelta(days=7))
    created: list[NewBlock] = [c[1] for c in repo.calls if c[0] == "create_generated"]
    assert [b.plan_date.weekday() for b in created] == [4, 6]  # Friday, Sunday
    friday = created[0]
    assert friday.started_at == datetime(2026, 5, 8, 9, tzinfo=MADRID)
    assert (friday.task_id, friday.label, friday.commitment_id) == (42, "Work", 7)
    assert repo.calls[0] == ("list_active_commitments",)


async def test_busy_hours_generate_first_and_pass_an_inclusive_end() -> None:
    repo = FakeRepo(list_active_commitments=[], sum_busy_hours_by_date={})
    await svc(repo, MADRID).busy_hours_by_date(date(2026, 5, 4), date(2026, 5, 11))
    assert repo.calls[-1] == (
        "sum_busy_hours_by_date",
        date(2026, 5, 4),
        date(2026, 5, 10),
        "Europe/Madrid",
    )


async def test_planned_hours_by_task() -> None:
    repo = FakeRepo(sum_planned_hours_by_task={1: Decimal(2)})
    assert await svc(repo).planned_hours_by_task([], NOW) == {}
    assert repo.calls == []
    assert await svc(repo).planned_hours_by_task([1], NOW) == {1: Decimal(2)}


# Commitments


def commitment_req(**data: Any) -> CreateCommitmentRequest:
    base = {
        "task_id": 42,
        "label": "Work",
        "days_of_week": [1, 2, 3, 4, 5],
        "start_time": "09:00",
        "end_time": "13:00",
    }
    return CreateCommitmentRequest.model_validate(base | data)


async def test_create_commitment() -> None:
    repo = FakeRepo(get_task_name="Work", create_commitment=commitment())
    assert (await svc(repo).create_commitment(commitment_req(label=" Work "))).label == "Work"
    assert repo.calls[1] == ("create_commitment", 42, "Work", [1, 2, 3, 4, 5], time(9), time(13))


@pytest.mark.parametrize(
    ("data", "err"),
    [
        ({"label": " "}, LabelRequiredError),
        ({"days_of_week": []}, DaysOfWeekRequiredError),
    ],
)
async def test_create_commitment_rejects(data: dict[str, Any], err: type[Exception]) -> None:
    with pytest.raises(err):
        await svc(FakeRepo()).create_commitment(commitment_req(**data))


async def test_create_commitment_unknown_task_and_bad_time() -> None:
    with pytest.raises(TaskNotFoundError):
        await svc(FakeRepo(get_task_name=TaskNotFoundError())).create_commitment(
            commitment_req(task_id=999)
        )
    with pytest.raises(ValueError):
        await svc(FakeRepo()).create_commitment(commitment_req(start_time="9h"))


async def test_update_commitment_time_change_reapplies_the_schedule() -> None:
    repo = FakeRepo(update_commitment=commitment(start_time="15:30"))
    await svc(repo, MADRID).update_commitment(7, UpdateCommitmentRequest(start_time="15:30"))
    assert repo.calls == [
        ("update_commitment", 7, {"start_time": time(15, 30)}),
        ("apply_commitment_schedule", 7, date(2026, 5, 10), "Europe/Madrid"),
    ]


async def test_update_commitment_pause_deletes_future_blocks() -> None:
    repo = FakeRepo(update_commitment=commitment(active=False))
    await svc(repo).update_commitment(7, UpdateCommitmentRequest(active=False))
    assert repo.calls[1] == ("delete_future_commitment_blocks", 7, NOW)


async def test_update_commitment_label_only_touches_no_blocks() -> None:
    repo = FakeRepo(update_commitment=commitment())
    req = UpdateCommitmentRequest(label=" Work ", end_time=None, active=True)
    await svc(repo).update_commitment(7, req)
    assert repo.calls == [("update_commitment", 7, {"label": "Work", "active": True})]


async def test_update_commitment_days_and_end_time() -> None:
    repo = FakeRepo(update_commitment=commitment())
    await svc(repo).update_commitment(
        7, UpdateCommitmentRequest(days_of_week=[2], end_time="14:00")
    )
    assert repo.calls[0][2] == {"days_of_week": [2], "end_time": time(14)}
    assert repo.names()[1] == "apply_commitment_schedule"


async def test_update_commitment_rejects() -> None:
    with pytest.raises(LabelRequiredError):
        await svc(FakeRepo()).update_commitment(7, UpdateCommitmentRequest(label=""))
    with pytest.raises(DaysOfWeekRequiredError):
        await svc(FakeRepo()).update_commitment(7, UpdateCommitmentRequest(days_of_week=[]))


async def test_list_and_delete_commitment() -> None:
    repo = FakeRepo(list_commitments=[commitment()])
    assert len(await svc(repo).list_commitments()) == 1
    await svc(repo).delete_commitment(7)
    assert repo.calls[1:] == [
        ("delete_future_commitment_blocks", 7, NOW),
        ("delete_commitment", 7),
    ]


def test_helpers() -> None:
    assert parse_hhmm("9:05") == time(9, 5)
    assert [weekday(date(2026, 5, d)) for d in (3, 4, 9)] == [0, 1, 6]
