package tasks

import "time"

type CreateProjectRequest struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	ParentID    *int32     `json:"parent_id"`
}

type ProjectResponse struct {
	ID          int32      `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	ParentID    *int32     `json:"parent_id"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

type CreateTaskRequest struct {
	ProjectID   *int32     `json:"project_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
}

type TaskResponse struct {
	ID          int32      `json:"id"`
	ProjectID   *int32     `json:"project_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

type CreateTodoRequest struct {
	TaskID int32  `json:"task_id"`
	Name   string `json:"name"`
}

type TodoResponse struct {
	ID     int32  `json:"id"`
	TaskID int32  `json:"task_id"`
	Name   string `json:"name"`
}

type CreateTimeEntryRequest struct {
	TaskID     int32      `json:"task_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Comment    *string    `json:"comment"`
}

type TimeEntryResponse struct {
	ID         int32      `json:"id"`
	TaskID     int32      `json:"task_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Comment    *string    `json:"comment"`
}

type FinishTimeEntryRequest struct {
	ID         int32      `json:"-"`
	FinishedAt *time.Time `json:"finished_at"`
}

type FinishTaskRequest struct {
	ID         int32      `json:"-"`
	FinishedAt *time.Time `json:"finished_at"`
}

type FinishProjectRequest struct {
	ID         int32      `json:"-"`
	FinishedAt *time.Time `json:"finished_at"`
}
