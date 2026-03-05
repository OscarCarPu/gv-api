package tasks

import (
	"context"
	"time"
)

type Service struct {
	repo     Repository
	location *time.Location
}

func NewService(repo Repository, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{repo: repo, location: loc}
}

func (s *Service) CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error) {
	return s.repo.CreateProject(ctx, req.Name, req.Description, req.DueAt, req.ParentID)
}

func (s *Service) CreateTask(ctx context.Context, req CreateTaskRequest) (TaskResponse, error) {
	return s.repo.CreateTask(ctx, req.ProjectID, req.Name, req.Description, req.DueAt)
}

func (s *Service) CreateTodo(ctx context.Context, req CreateTodoRequest) (TodoResponse, error) {
	return s.repo.CreateTodo(ctx, req.TaskID, req.Name)
}

func (s *Service) CreateTimeEntry(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error) {
	return s.repo.CreateTimeEntry(ctx, req.TaskID, req.StartedAt, req.FinishedAt, req.Comment)
}

func (s *Service) FinishTimeEntry(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error) {
	finishedAt := time.Now().In(s.location)
	if req.FinishedAt != nil {
		finishedAt = *req.FinishedAt
	}
	return s.repo.FinishTimeEntry(ctx, req.ID, finishedAt)
}

func (s *Service) FinishTask(ctx context.Context, req FinishTaskRequest) (TaskResponse, error) {
	finishedAt := time.Now().In(s.location)
	if req.FinishedAt != nil {
		finishedAt = *req.FinishedAt
	}
	return s.repo.FinishTask(ctx, req.ID, finishedAt)
}

func (s *Service) FinishProject(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error) {
	finishedAt := time.Now().In(s.location)
	if req.FinishedAt != nil {
		finishedAt = *req.FinishedAt
	}
	return s.repo.FinishProject(ctx, req.ID, finishedAt)
}

func (s *Service) GetActiveTree(ctx context.Context) ([]ActiveTreeNode, error) {
	return s.repo.GetActiveTree(ctx)
}

func (s *Service) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	return s.repo.GetRootProjects(ctx)
}
