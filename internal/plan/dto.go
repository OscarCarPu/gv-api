package plan

import (
	"time"

	"gv-api/internal/tasks"
)

type PlanBlockResponse struct {
	ID             int32      `json:"id"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        time.Time  `json:"ended_at"`
	TaskID         *int32     `json:"task_id"`
	TaskName       *string    `json:"task_name"`
	Label          string     `json:"label"`
	Note           *string    `json:"note"`
	TaskType       *string    `json:"task_type"`
	TaskRecurrence *int32     `json:"task_recurrence"`
	TaskStartedAt  *time.Time `json:"task_started_at"`
	TaskFinishedAt *time.Time `json:"task_finished_at"`
}

type PlanTotals struct {
	TaskSeconds int64 `json:"task_seconds"`
	FreeSeconds int64 `json:"free_seconds"`
}

type PlanTodayResponse struct {
	Date   string                         `json:"date"`
	Blocks []PlanBlockResponse            `json:"blocks"`
	Totals PlanTotals                     `json:"totals"`
	Budget tasks.TimeEntrySummaryResponse `json:"budget"`
}

type CreatePlanBlockRequest struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	TaskID    *int32    `json:"task_id"`
	Label     *string   `json:"label"`
	Note      *string   `json:"note"`
}

type UpdatePlanBlockRequest struct {
	ID        int32      `json:"-"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	TaskID    *int32     `json:"task_id"`
	ClearTask bool       `json:"clear_task"`
	Label     *string    `json:"label"`
	Note      *string    `json:"note"`
	ClearNote bool       `json:"clear_note"`
}
