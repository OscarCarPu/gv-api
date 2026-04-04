package tasks

import (
	"encoding/json"
	"time"
)

type TaskDepRef struct {
	ID    int32   `json:"id"`
	Name  string  `json:"name"`
	DueAt *string `json:"due_at"`
}

func unmarshalDepRefs(data []byte) []TaskDepRef {
	if len(data) == 0 {
		return []TaskDepRef{}
	}
	var refs []TaskDepRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return []TaskDepRef{}
	}
	if refs == nil {
		return []TaskDepRef{}
	}
	return refs
}

func depRefIDs(refs []TaskDepRef) []int32 {
	ids := make([]int32, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return ids
}

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

type ProjectFastResponse struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type TaskFastResponse struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type CreateTaskRequest struct {
	ProjectID   *int32     `json:"project_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	DependsOn   []int32    `json:"depends_on"`
}

type TaskResponse struct {
	ID          int32        `json:"id"`
	ProjectID   *int32       `json:"project_id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	DueAt       *time.Time   `json:"due_at"`
	StartedAt   *time.Time   `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at"`
	DependsOn []TaskDepRef `json:"depends_on"`
	Blocks    []TaskDepRef `json:"blocks"`
	Blocked   bool         `json:"blocked"`
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

type UpdateProjectRequest struct {
	ID          int32      `json:"-"`
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	ParentID    *int32     `json:"parent_id"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

// NullableTime distinguishes between an absent JSON field and an explicit null.
// Set is true when the field was present in the JSON payload (even if null).
type NullableTime struct {
	Value *time.Time
	Set   bool
}

func (n *NullableTime) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	n.Value = &t
	return nil
}

type UpdateTaskRequest struct {
	ID          int32        `json:"-"`
	Name        *string      `json:"name"`
	Description *string      `json:"description"`
	DueAt       NullableTime `json:"due_at"`
	ProjectID   *int32       `json:"project_id"`
	StartedAt   *time.Time   `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at"`
	DependsOn   *[]int32     `json:"depends_on"`
}

type UpdateTodoRequest struct {
	ID     int32   `json:"-"`
	TaskID *int32  `json:"task_id"`
	Name   *string `json:"name"`
	IsDone *bool   `json:"is_done"`
}

type UpdateTimeEntryRequest struct {
	ID         int32      `json:"-"`
	TaskID     *int32     `json:"task_id"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Comment    *string    `json:"comment"`
}

type TaskTimeEntriesResponse struct {
	Task        TaskDetailResponse  `json:"task"`
	TimeEntries []TimeEntryResponse `json:"time_entries"`
}

type TaskDetailResponse struct {
	ID          int32        `json:"id"`
	ProjectID   *int32       `json:"project_id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	DueAt       *time.Time   `json:"due_at"`
	StartedAt   *time.Time   `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at"`
	TimeSpent   int64        `json:"time_spent"`
	DependsOn []TaskDepRef `json:"depends_on"`
	Blocks    []TaskDepRef `json:"blocks"`
	Blocked   bool         `json:"blocked"`
}

type TaskFullResponse struct {
	ID          int32          `json:"id"`
	ProjectID   *int32         `json:"project_id"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	DueAt       *time.Time     `json:"due_at"`
	StartedAt   *time.Time     `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at"`
	TimeSpent   int64          `json:"time_spent"`
	DependsOn []TaskDepRef   `json:"depends_on"`
	Blocks    []TaskDepRef   `json:"blocks"`
	Blocked   bool           `json:"blocked"`
	Todos     []TodoResponse `json:"todos"`
}

type TaskByDueDateResponse struct {
	ID           int32        `json:"id"`
	Name         string       `json:"name"`
	Description  *string      `json:"description"`
	DueAt        *time.Time   `json:"due_at"`
	StartedAt    *time.Time   `json:"started_at"`
	TimeSpent    int64        `json:"time_spent"`
	ProjectID    *int32       `json:"project_id"`
	ProjectName  *string      `json:"project_name"`
	ProjectDueAt *time.Time   `json:"project_due_at"`
	DependsOn []TaskDepRef `json:"depends_on"`
	Blocks    []TaskDepRef `json:"blocks"`
	Blocked   bool         `json:"blocked"`
}

type TimeEntrySummaryResponse struct {
	Today int64 `json:"today"`
	Week  int64 `json:"week"`
}

type ActiveProject struct {
	ID       int32
	ParentID *int32
	Name     string
	DueAt    *time.Time
}

type UnfinishedTask struct {
	ID          int32
	ProjectID   *int32
	Name        string
	Description *string
	DueAt       *time.Time
	Started     bool
	StartedAt   *time.Time
	DependsOn   []TaskDepRef
	Blocks      []TaskDepRef
}

type ActiveTreeNode struct {
	ID          int32            `json:"id"`
	Type        string           `json:"type"`
	Name        string           `json:"name"`
	Description *string          `json:"description,omitempty"`
	DueAt       *time.Time       `json:"due_at,omitempty"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	DependsOn []TaskDepRef     `json:"depends_on"`
	Blocks    []TaskDepRef     `json:"blocks"`
	Blocked   bool             `json:"blocked"`
	Children  []ActiveTreeNode `json:"children,omitempty"`
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
	DependsOn []TaskDepRef   `json:"depends_on,omitempty"`
	Blocks    []TaskDepRef   `json:"blocks,omitempty"`
	Blocked   *bool          `json:"blocked,omitempty"`
	Todos     []TodoResponse `json:"todos,omitempty"`
}
