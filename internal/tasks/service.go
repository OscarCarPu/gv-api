package tasks

import (
	"context"
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
	resp, err := s.repo.CreateTask(ctx, req.ProjectID, req.Name, req.Description, req.DueAt)
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

func (s *Service) GetActiveTree(ctx context.Context) ([]ActiveTreeNode, error) {
	projects, err := s.repo.GetActiveProjects(ctx)
	if err != nil {
		return nil, err
	}

	allTasks, err := s.repo.GetUnfinishedTasks(ctx)
	if err != nil {
		return nil, err
	}

	// Build dependency lookup structures
	unfinishedIDs := make(map[int32]bool, len(allTasks))
	tasksByID := make(map[int32]UnfinishedTask, len(allTasks))
	for _, t := range allTasks {
		unfinishedIDs[t.ID] = true
		tasksByID[t.ID] = t
	}

	// Filter out hidden tasks (all deps are themselves blocked)
	var tasks []UnfinishedTask
	for _, t := range allTasks {
		if !isHidden(t, tasksByID, unfinishedIDs) {
			tasks = append(tasks, t)
		}
	}

	// Compute effective due dates (memoized)
	memo := make(map[int32]*time.Time)

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
			DueAt:       effectiveDueDate(t.ID, tasksByID, memo, make(map[int32]bool)),
			StartedAt:   t.StartedAt,
			DependsOn:   t.DependsOn,
			Blocks:      t.Blocks,
			Blocked:     isBlocked(t, unfinishedIDs),
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

// isBlocked returns true if the task has at least one unfinished dependency.
func isBlocked(t UnfinishedTask, unfinishedIDs map[int32]bool) bool {
	for _, dep := range t.DependsOn {
		if unfinishedIDs[dep.ID] {
			return true
		}
	}
	return false
}

// isHidden returns true if ALL of the task's dependencies are themselves blocked.
// A task with no dependencies is never hidden.
func isHidden(t UnfinishedTask, tasksByID map[int32]UnfinishedTask, unfinishedIDs map[int32]bool) bool {
	if len(t.DependsOn) == 0 {
		return false
	}
	for _, dep := range t.DependsOn {
		d, exists := tasksByID[dep.ID]
		if !exists {
			// Dependency is finished → not blocked → task is not hidden
			return false
		}
		if !isBlocked(d, unfinishedIDs) {
			return false
		}
	}
	return true
}

// effectiveDueDate computes min(own due_at, effective due_at of each dependency).
// Uses memoization to avoid recomputation. visited prevents infinite loops.
func effectiveDueDate(taskID int32, tasksByID map[int32]UnfinishedTask, memo map[int32]*time.Time, visited map[int32]bool) *time.Time {
	if result, ok := memo[taskID]; ok {
		return result
	}
	if visited[taskID] {
		return nil
	}
	visited[taskID] = true

	t, exists := tasksByID[taskID]
	if !exists {
		return nil
	}

	best := t.DueAt
	for _, dep := range t.DependsOn {
		depDue := effectiveDueDate(dep.ID, tasksByID, memo, visited)
		if depDue != nil && (best == nil || depDue.Before(*best)) {
			best = depDue
		}
	}

	memo[taskID] = best
	return best
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
	rows, err := s.repo.GetTasksByDueDate(ctx)
	if err != nil {
		return nil, err
	}

	// Build lookup structures for dependency logic
	type taskDep struct {
		DependsOn []TaskDepRef
		DueAt     *time.Time
	}
	unfinishedIDs := make(map[int32]bool, len(rows))
	depsByID := make(map[int32]taskDep, len(rows))
	for _, r := range rows {
		unfinishedIDs[r.ID] = true
		depsByID[r.ID] = taskDep{DependsOn: r.DependsOn, DueAt: r.DueAt}
	}

	// Compute effective due dates (memoized)
	memo := make(map[int32]*time.Time)
	var effDue func(id int32, visited map[int32]bool) *time.Time
	effDue = func(id int32, visited map[int32]bool) *time.Time {
		if result, ok := memo[id]; ok {
			return result
		}
		if visited[id] {
			return nil
		}
		visited[id] = true

		d, exists := depsByID[id]
		if !exists {
			return nil
		}
		best := d.DueAt
		for _, dep := range d.DependsOn {
			depDueAt := effDue(dep.ID, visited)
			if depDueAt != nil && (best == nil || depDueAt.Before(*best)) {
				best = depDueAt
			}
		}
		memo[id] = best
		return best
	}

	// isBlockedDue checks if a task has at least one unfinished dependency
	isBlockedDue := func(deps []TaskDepRef) bool {
		for _, dep := range deps {
			if unfinishedIDs[dep.ID] {
				return true
			}
		}
		return false
	}

	// isHiddenDue checks if ALL of the task's deps are themselves blocked
	isHiddenDue := func(deps []TaskDepRef) bool {
		if len(deps) == 0 {
			return false
		}
		for _, dep := range deps {
			d, exists := depsByID[dep.ID]
			if !exists || !isBlockedDue(d.DependsOn) {
				return false
			}
		}
		return true
	}

	var result []TaskByDueDateResponse
	for _, r := range rows {
		if isHiddenDue(r.DependsOn) {
			continue
		}

		eDue := effDue(r.ID, make(map[int32]bool))

		// Task qualifies if it has: own due_at, project due_at, or effective due_at from deps
		if r.DueAt == nil && r.ProjectDueAt == nil && eDue == nil {
			continue
		}

		// Use effective due date if it's earlier than own
		if eDue != nil && (r.DueAt == nil || eDue.Before(*r.DueAt)) {
			r.DueAt = eDue
		}

		r.Blocked = isBlockedDue(r.DependsOn)
		result = append(result, r)
	}

	// Re-sort by effective due date since due_at may have changed
	sort.Slice(result, func(i, j int) bool {
		di, dj := result[i].DueAt, result[j].DueAt
		pi, pj := result[i].ProjectDueAt, result[j].ProjectDueAt

		// due_at ASC NULLS LAST
		if di != nil && dj != nil {
			if !di.Equal(*dj) {
				return di.Before(*dj)
			}
		} else if di != nil {
			return true
		} else if dj != nil {
			return false
		}

		// project_due_at ASC NULLS LAST
		if pi != nil && pj != nil {
			if !pi.Equal(*pj) {
				return pi.Before(*pj)
			}
		} else if pi != nil {
			return true
		} else if pj != nil {
			return false
		}

		// name ASC
		return result[i].Name < result[j].Name
	})

	if result == nil {
		result = []TaskByDueDateResponse{}
	}
	return result, nil
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

	data = history.FillMissingPeriods(data, start, end, frequency)

	return history.Response{
		StartAt: start.Format("2006-01-02"),
		EndAt:   end.Format("2006-01-02"),
		Data:    data,
	}, nil
}
