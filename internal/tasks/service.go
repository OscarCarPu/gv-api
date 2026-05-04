package tasks

import (
	"context"
	"fmt"
	"sort"
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
	taskType := "standard"
	if req.TaskType != nil && *req.TaskType != "" {
		taskType = *req.TaskType
	}
	priority := int32(3)
	if req.Priority != nil {
		priority = *req.Priority
	}
	resp, err := s.repo.CreateTask(ctx, req.ProjectID, req.Name, req.Description, req.DueAt, taskType, req.Recurrence, priority)
	if err != nil {
		return resp, err
	}

	if len(req.DependsOn) > 0 {
		if err := s.repo.ReplaceTaskDependencies(ctx, resp.ID, req.DependsOn); err != nil {
			return resp, err
		}
	}

	dependsOn, blocks, blocked, err := s.repo.GetTaskDependencies(ctx, resp.ID)
	if err != nil {
		return resp, err
	}
	resp.DependsOn = dependsOn
	resp.Blocks = blocks
	resp.Blocked = blocked

	return resp, nil
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
	resp, err := s.repo.UpdateTask(ctx, req)
	if err != nil {
		return resp, err
	}

	if req.DependsOn != nil {
		if err := s.repo.ReplaceTaskDependencies(ctx, resp.ID, *req.DependsOn); err != nil {
			return resp, err
		}
	}
	if req.Blocks != nil {
		if err := s.repo.ReplaceTaskBlocks(ctx, resp.ID, *req.Blocks); err != nil {
			return resp, err
		}
	}

	dependsOn, blocks, blocked, err := s.repo.GetTaskDependencies(ctx, resp.ID)
	if err != nil {
		return resp, err
	}
	resp.DependsOn = dependsOn
	resp.Blocks = blocks
	resp.Blocked = blocked

	return resp, nil
}

func (s *Service) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	return s.repo.UpdateTodo(ctx, req)
}

func (s *Service) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	return s.repo.UpdateTimeEntry(ctx, req)
}

func (s *Service) GetActiveTree(ctx context.Context, minPriority *int32) ([]ActiveTreeNode, error) {
	projects, err := s.repo.GetActiveProjects(ctx)
	if err != nil {
		return nil, err
	}

	tasks, err := s.repo.GetUnfinishedTasks(ctx, minPriority)
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
		taskType := t.TaskType
		priority := t.Priority
		node := ActiveTreeNode{
			ID:          t.ID,
			Type:        "task",
			Name:        t.Name,
			Description: t.Description,
			DueAt:       t.DueAt,
			StartedAt:   t.StartedAt,
			TaskType:    &taskType,
			Recurrence:  t.Recurrence,
			Priority:    &priority,
			DependsOn:   t.DependsOn,
			Blocks:      t.Blocks,
			Blocked:     t.Blocked,
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

	// Compute depth for each project so we attach deepest children first.
	// This ensures that when a project is copied into its parent, all its
	// own children are already attached.
	depthOf := make(map[int32]int, len(projects))
	parentOf := make(map[int32]*int32, len(projects))
	for _, p := range projects {
		parentOf[p.ID] = p.ParentID
	}
	var getDepth func(id int32) int
	getDepth = func(id int32) int {
		if d, ok := depthOf[id]; ok {
			return d
		}
		pid := parentOf[id]
		if pid == nil {
			depthOf[id] = 0
			return 0
		}
		if _, ok := parentOf[*pid]; !ok {
			depthOf[id] = 0
			return 0
		}
		d := getDepth(*pid) + 1
		depthOf[id] = d
		return d
	}
	for _, p := range projects {
		getDepth(p.ID)
	}

	// Sort projects by depth descending so deepest nest first
	sorted := make([]ActiveProject, len(projects))
	copy(sorted, projects)
	sort.Slice(sorted, func(i, j int) bool {
		return depthOf[sorted[i].ID] > depthOf[sorted[j].ID]
	})

	// Attach child projects to parent projects (sub-projects first, before tasks)
	childProjectIDs := make(map[int32]bool)
	for _, p := range sorted {
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

func (s *Service) GetTasksByDueDate(ctx context.Context, minPriority *int32) ([]TaskByDueDateResponse, error) {
	rows, err := s.repo.GetTasksByDueDate(ctx, minPriority)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []TaskByDueDateResponse{}
	}
	return rows, nil
}

func (s *Service) GetActiveTimeEntry(ctx context.Context) (ActiveTimeEntryResponse, error) {
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

func (s *Service) ListTasksFast(ctx context.Context) ([]TaskFastResponse, error) {
	return s.repo.ListTasksFast(ctx)
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

	return history.Response{
		StartAt: start.Format("2006-01-02"),
		EndAt:   end.Format("2006-01-02"),
		Data:    data,
	}, nil
}

func (s *Service) GetTimeEntriesByDateRange(ctx context.Context, startTime, endTime string) ([]TimeEntryWithTaskResponse, error) {
	start, err := time.Parse("2006-01-02", startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time format")
	}

	var end time.Time
	if endTime != "" {
		end, err = time.Parse("2006-01-02", endTime)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time format")
		}
	} else {
		now := time.Now().In(s.location)
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	}

	startTS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, s.location)
	endTS := time.Date(end.Year(), end.Month(), end.Day()+1, 0, 0, 0, 0, s.location)

	entries, err := s.repo.GetTimeEntriesByDateRange(ctx, startTS, endTS)
	if err != nil {
		return nil, err
	}

	if entries == nil {
		entries = []TimeEntryWithTaskResponse{}
	}

	return entries, nil
}
