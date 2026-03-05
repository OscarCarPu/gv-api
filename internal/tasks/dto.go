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
	IsDone bool   `json:"is_done"`
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

type TaskTimeEntriesResponse struct {
	Task        TaskDetailResponse  `json:"task"`
	TimeEntries []TimeEntryResponse `json:"time_entries"`
}

type TaskDetailResponse struct {
	ID          int32      `json:"id"`
	ProjectID   *int32     `json:"project_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	TimeSpent   int64      `json:"time_spent"`
}

type ActiveProject struct {
	ID       int32
	ParentID *int32
	Name     string
}

type UnfinishedTask struct {
	ID        int32
	ProjectID *int32
	Name      string
	Started   bool
}

type ActiveTreeNode struct {
	ID       int32            `json:"id"`
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	Children []ActiveTreeNode `json:"children,omitempty"`
}

type ProjectChildrenResponse struct {
	Project  ProjectDetailResponse `json:"project"`
	Children []ProjectChildNode    `json:"children"`
}

type ProjectDetailResponse struct {
	ID          int32      `json:"id"`
	ParentID    *int32     `json:"parent_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	TimeSpent   int64      `json:"time_spent"`
}

type ProjectChildNode struct {
	ID          int32      `json:"id"`
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	TimeSpent   int64      `json:"time_spent"`

	// Project-only
	ParentID *int32 `json:"parent_id,omitempty"`

	// Task-only
	ProjectID *int32         `json:"project_id,omitempty"`
	Todos     []TodoResponse `json:"todos,omitempty"`
}
