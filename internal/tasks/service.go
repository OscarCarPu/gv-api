package tasks

import (
	"context"
	"sort"
	"time"

	"gv-api/internal/capacity"
	"gv-api/internal/history"

	"github.com/shopspring/decimal"
)

type capacityProvider interface {
	FreeBusyRange(ctx context.Context, from, to time.Time) ([]capacity.DayFreeBusy, error)
}

type plannedHoursProvider interface {
	PlannedHoursByTask(ctx context.Context, taskIDs []int32, from time.Time) (map[int32]decimal.Decimal, error)
}

type Service struct {
	repo     Repository
	location *time.Location
	capacity capacityProvider
	plan     plannedHoursProvider
}

func NewService(repo Repository, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{repo: repo, location: loc}
}

// SetUrgencyProviders wires the dependencies GetTasksByDueDate needs to compute urgency.
// A setter rather than a constructor argument because plan.Service itself depends on
// tasks.Service (for the time-entry budget summary) — constructing both the normal way would
// require each to exist before the other does.
func (s *Service) SetUrgencyProviders(capacity capacityProvider, plan plannedHoursProvider) {
	s.capacity = capacity
	s.plan = plan
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
	resp, err := s.repo.CreateTask(ctx, req.ProjectID, req.Name, req.Description, req.DueAt, taskType, req.Recurrence, priority, req.EstimateHours)
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

	if req.FinishedAt.Set && req.FinishedAt.Value != nil {
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

	// Group tasks by project ID
	projectTasks := make(map[int32][]ActiveTreeNode)
	var orphanTasks []ActiveTreeNode

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
				projectTasks[*t.ProjectID] = append(projectTasks[*t.ProjectID], node)
			}
			// project not active — skip task
			continue
		}
		orphanTasks = append(orphanTasks, node)
	}

	// Attach tasks to each project node (SQL already orders: en progreso → continua → recurrente → pendiente)
	for id, node := range projectNodes {
		node.Children = append(node.Children, projectTasks[id]...)
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
	root = append(root, orphanTasks...)

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
	if err := s.applyUrgency(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func effectiveDue(t TaskByDueDateResponse) *time.Time {
	if t.DueAt != nil {
		return t.DueAt
	}
	return t.ProjectDueAt
}

// normalizedDue re-anchors a due date's calendar day to s.location's midnight. due_at is stored
// as a conceptual date (midnight UTC), not a real moment; comparing it as-is against "today"
// (already midnight in s.location) mixes two different offsets for the same calendar day and
// puts the day boundary in the wrong place by exactly that offset.
func (s *Service) normalizedDue(t TaskByDueDateResponse) *time.Time {
	due := effectiveDue(t)
	if due == nil {
		return nil
	}
	norm := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, s.location)
	return &norm
}

// applyUrgency fills RemainingHours/StartBy/Urgent on standard and recurring tasks that carry an
// estimate. Continuous tasks are skipped: they have no real deadline to count back from.
// Recurring tasks are included, but never have spentHours subtracted from their estimate — see
// the note at that line for why.
func (s *Service) applyUrgency(ctx context.Context, rows []TaskByDueDateResponse) error {
	if s.capacity == nil || s.plan == nil {
		return nil
	}

	today := time.Now().In(s.location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, s.location)

	var maxDue time.Time
	estimatedIdx := make([]int, 0, len(rows))
	estimatedIDs := make([]int32, 0, len(rows))
	for i, t := range rows {
		if (t.TaskType != "standard" && t.TaskType != "recurring") || t.EstimateHours == nil {
			continue
		}
		due := s.normalizedDue(t)
		if due == nil {
			continue
		}
		estimatedIdx = append(estimatedIdx, i)
		estimatedIDs = append(estimatedIDs, t.ID)
		if due.After(maxDue) {
			maxDue = *due
		}
	}
	if maxDue.IsZero() {
		return nil
	}

	series, err := s.capacity.FreeBusyRange(ctx, today, maxDue.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	freeByDate := make(map[string]decimal.Decimal, len(series))
	for _, d := range series {
		freeByDate[d.Date] = d.FreeHours
	}

	plannedByTask, err := s.plan.PlannedHoursByTask(ctx, estimatedIDs, today)
	if err != nil {
		return err
	}

	// Every task's backward-fill draws from the same freeByDate pool, so left unordered two
	// tasks racing for the same day's hours would each assume they had all of it to themselves.
	// Higher-priority tasks claim hours first and decrement the shared pool as they go, so a
	// lower-priority task genuinely starved by that competition shows up as urgent even though
	// it would easily fit in the capacity if it were the only task due around then. Priority
	// wins the claim order; the sooner deadline breaks ties within the same priority.
	sort.SliceStable(estimatedIdx, func(a, b int) bool {
		ra, rb := &rows[estimatedIdx[a]], &rows[estimatedIdx[b]]
		if ra.Priority != rb.Priority {
			return ra.Priority < rb.Priority
		}
		return s.normalizedDue(*ra).Before(*s.normalizedDue(*rb))
	})

	for _, i := range estimatedIdx {
		t := &rows[i]
		due := s.normalizedDue(*t)

		// A recurring task's time_spent accumulates across every past cycle — renewing only
		// reschedules due_at, it never resets time_spent — so subtracting it here would read
		// as permanently over-estimate after a couple of renewals. estimate_hours is a
		// per-cycle target, not a lifetime one, for this task type.
		spentHours := decimal.Zero
		if t.TaskType != "recurring" {
			spentHours = decimal.NewFromInt(t.TimeSpent).Div(decimal.NewFromInt(3600))
		}
		remaining := t.EstimateHours.Sub(spentHours).Sub(plannedByTask[t.ID])
		if remaining.IsNegative() {
			remaining = decimal.Zero
		}
		t.RemainingHours = &remaining
		if remaining.IsZero() {
			continue
		}

		acc := decimal.Zero
		startBy := today
		consumed := make(map[string]decimal.Decimal)
		for d := due.AddDate(0, 0, -1); !d.Before(today); d = d.AddDate(0, 0, -1) {
			dateStr := d.Format("2006-01-02")
			if avail := freeByDate[dateStr]; avail.GreaterThan(decimal.Zero) {
				take := avail
				if need := remaining.Sub(acc); take.GreaterThan(need) {
					take = need
				}
				acc = acc.Add(take)
				consumed[dateStr] = take
			}
			if acc.GreaterThanOrEqual(remaining) {
				startBy = d
				break
			}
		}
		for dateStr, amt := range consumed {
			freeByDate[dateStr] = freeByDate[dateStr].Sub(amt)
		}

		startByStr := startBy.Format("2006-01-02")
		t.StartBy = &startByStr
		t.Urgent = !startBy.After(today)
	}
	return nil
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

	summary, err := s.repo.GetTimeEntrySummary(ctx, todayStart, weekStart)
	if err != nil {
		return summary, err
	}

	summary.WeeklyTargetSeconds = WeeklyTaskTargetSeconds
	summary.Pace = CalcPace(now, summary.Week, summary.Today)
	summary.DailyTargetSeconds = CalcDailyTargetSeconds(now, summary.Week, summary.Today)
	return summary, nil
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

func (s *Service) GetTimeEntriesByDateRange(ctx context.Context, start time.Time, end *time.Time) ([]TimeEntryWithTaskResponse, error) {
	var endDate time.Time
	if end != nil {
		endDate = *end
	} else {
		now := time.Now().In(s.location)
		endDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	}

	startTS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, s.location)
	endTS := time.Date(endDate.Year(), endDate.Month(), endDate.Day()+1, 0, 0, 0, 0, s.location)

	entries, err := s.repo.GetTimeEntriesByDateRange(ctx, startTS, endTS)
	if err != nil {
		return nil, err
	}

	if entries == nil {
		entries = []TimeEntryWithTaskResponse{}
	}

	return entries, nil
}
