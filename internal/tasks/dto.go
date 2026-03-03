package tasks

import "time"

type CreateProjectRequest struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	ParentID    *int32     `json:"parent_id"`
}

type CreateProjectResponse struct {
	ID          int32      `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	ParentID    *int32     `json:"parent_id"`
}
