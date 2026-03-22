package tasks

import (
	"context"
	"time"

	"gv-api/internal/history"
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

func (s *Service) UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
	resp, err := s.repo.UpdateProject(ctx, req)
	if err != nil {
		return resp, err
	}

	if req.FinishedAt != nil {
		if err := s.repo.FinishDescendantProjects(ctx, req.ID); err != nil {
			return resp, err
		}
		if err := s.repo.FinishTasksByProjectTree(ctx, req.ID); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

func (s *Service) UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
	return s.repo.UpdateTask(ctx, req)
}

func (s *Service) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	return s.repo.UpdateTodo(ctx, req)
}

func (s *Service) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	return s.repo.UpdateTimeEntry(ctx, req)
}

func (s *Service) GetActiveTree(ctx context.Context) ([]ActiveTreeNode, error) {
	projects, err := s.repo.GetActiveProjects(ctx)
	if err != nil {
		return nil, err
	}

	tasks, err := s.repo.GetUnfinishedTasks(ctx)
	if err != nil {
		return nil, err
	}

	// Build project nodes indexed by ID
	projectNodes := make(map[int32]*ActiveTreeNode, len(projects))
	for _, p := range projects {
		projectNodes[p.ID] = &ActiveTreeNode{
			ID:       p.ID,
			Type:     "project",
			Name:     p.Name,
			DueAt:    p.DueAt,
			Children: []ActiveTreeNode{},
		}
	}

	// Group tasks by project ID, separating started vs unstarted
	startedTasks := make(map[int32][]ActiveTreeNode)
	unstartedTasks := make(map[int32][]ActiveTreeNode)
	var orphanStarted []ActiveTreeNode
	var orphanUnstarted []ActiveTreeNode

	for _, t := range tasks {
		node := ActiveTreeNode{
			ID:          t.ID,
			Type:        "task",
			Name:        t.Name,
			Description: t.Description,
			DueAt:       t.DueAt,
			StartedAt:   t.StartedAt,
		}

		if t.ProjectID != nil {
			if _, ok := projectNodes[*t.ProjectID]; ok {
				if t.Started {
					startedTasks[*t.ProjectID] = append(startedTasks[*t.ProjectID], node)
				} else {
					unstartedTasks[*t.ProjectID] = append(unstartedTasks[*t.ProjectID], node)
				}
			}
			continue
		}
		if t.Started {
			orphanStarted = append(orphanStarted, node)
		} else {
			orphanUnstarted = append(orphanUnstarted, node)
		}
	}

	// Attach tasks to each project node
	for id, node := range projectNodes {
		node.Children = append(node.Children, startedTasks[id]...)
		node.Children = append(node.Children, unstartedTasks[id]...)
	}

	// Attach child projects to parent projects (sub-projects first, before tasks)
	childProjectIDs := make(map[int32]bool)
	for _, p := range projects {
		if p.ParentID != nil {
			if parent, ok := projectNodes[*p.ParentID]; ok {
				childProjectIDs[p.ID] = true
				parent.Children = append([]ActiveTreeNode{*projectNodes[p.ID]}, parent.Children...)
			}
		}
	}

	// Build root: projects that aren't children, then orphan tasks
	var root []ActiveTreeNode
	for _, p := range projects {
		if !childProjectIDs[p.ID] {
			root = append(root, *projectNodes[p.ID])
		}
	}
	root = append(root, orphanStarted...)
	root = append(root, orphanUnstarted...)

	if root == nil {
		root = []ActiveTreeNode{}
	}

	return root, nil
}

func (s *Service) GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error) {
	return s.repo.GetProject(ctx, id)
}

func (s *Service) GetTask(ctx context.Context, id int32) (TaskFullResponse, error) {
	return s.repo.GetTask(ctx, id)
}

func (s *Service) GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
	return s.repo.GetProjectChildren(ctx, projectID)
}

func (s *Service) GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
	return s.repo.GetTaskTimeEntries(ctx, taskID)
}

func (s *Service) GetTasksByDueDate(ctx context.Context) ([]TaskByDueDateResponse, error) {
	return s.repo.GetTasksByDueDate(ctx)
}

func (s *Service) GetActiveTimeEntry(ctx context.Context) (TimeEntryResponse, error) {
	return s.repo.GetActiveTimeEntry(ctx)
}

func (s *Service) GetTimeEntrySummary(ctx context.Context) (TimeEntrySummaryResponse, error) {
	now := time.Now().In(s.location)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)

	// Find last Monday at 00:00
	weekday := now.Weekday()
	daysSinceMonday := (int(weekday) + 6) % 7 // Monday=0, Sunday=6
	weekStart := time.Date(now.Year(), now.Month(), now.Day()-daysSinceMonday, 0, 0, 0, 0, s.location)

	return s.repo.GetTimeEntrySummary(ctx, todayStart, weekStart)
}

func (s *Service) ListProjectsFast(ctx context.Context) ([]ProjectFastResponse, error) {
	return s.repo.ListProjectsFast(ctx)
}

func (s *Service) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	return s.repo.GetRootProjects(ctx)
}

func (s *Service) DeleteProject(ctx context.Context, id int32) error {
	return s.repo.DeleteProject(ctx, id)
}

func (s *Service) DeleteTask(ctx context.Context, id int32) error {
	return s.repo.DeleteTask(ctx, id)
}

func (s *Service) DeleteTodo(ctx context.Context, id int32) error {
	return s.repo.DeleteTodo(ctx, id)
}

func (s *Service) DeleteTimeEntry(ctx context.Context, id int32) error {
	return s.repo.DeleteTimeEntry(ctx, id)
}

func (s *Service) GetTimeEntryHistory(ctx context.Context, frequency, startAt, endAt string) (history.Response, error) {
	trunc, err := history.ValidFrequency(frequency)
	if err != nil {
		return history.Response{}, err
	}

	start, end, err := history.ParseDateRange(s.location, frequency, startAt, endAt)
	if err != nil {
		return history.Response{}, err
	}

	data, err := s.repo.GetTimeEntryHistory(ctx, trunc, s.location.String(), start, end)
	if err != nil {
		return history.Response{}, err
	}

	data = history.FillMissingPeriods(data, start, end, frequency)

	return history.Response{
		StartAt: start.Format("2006-01-02"),
		EndAt:   end.Format("2006-01-02"),
		Data:    data,
	}, nil
}
