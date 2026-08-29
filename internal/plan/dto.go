package plan

import (
	"time"

	"gv-api/internal/tasks"
)

type PlanBlockResponse struct {
	ID             int32      `json:"id"`
	PlanDate       time.Time  `json:"plan_date"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        time.Time  `json:"ended_at"`
	TaskID         *int32     `json:"task_id"`
	TaskName       *string    `json:"task_name"`
	Label          string     `json:"label"`
	Note           *string    `json:"note"`
	EventRef       *string    `json:"event_ref"`
	CommitmentID   *int32     `json:"commitment_id"`
	TaskType       *string    `json:"task_type"`
	TaskRecurrence *int32     `json:"task_recurrence"`
	TaskStartedAt  *time.Time `json:"task_started_at"`
	TaskFinishedAt *time.Time `json:"task_finished_at"`
}

type PlanRangeResponse struct {
	From   string              `json:"from"`
	To     string              `json:"to"`
	Blocks []PlanBlockResponse `json:"blocks"`
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
	EventRef  *string   `json:"event_ref"`
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

	// ClearCommitmentID is never set from a request body — the service sets it when it
	// detects that editing started_at/ended_at moved the block to a different plan_date, so a
	// commitment-linked block does not silently duplicate on the next generation pass.
	ClearCommitmentID bool `json:"-"`
}

type RecurringCommitmentResponse struct {
	ID         int32   `json:"id"`
	TaskID     int32   `json:"task_id"`
	TaskName   string  `json:"task_name"`
	Label      string  `json:"label"`
	DaysOfWeek []int32 `json:"days_of_week"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Active     bool    `json:"active"`
}

type CreateCommitmentRequest struct {
	TaskID     int32   `json:"task_id"`
	Label      string  `json:"label"`
	DaysOfWeek []int32 `json:"days_of_week"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
}

type UpdateCommitmentRequest struct {
	ID         int32    `json:"-"`
	Label      *string  `json:"label"`
	DaysOfWeek *[]int32 `json:"days_of_week"`
	StartTime  *string  `json:"start_time"`
	EndTime    *string  `json:"end_time"`
	Active     *bool    `json:"active"`
}
