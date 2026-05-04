package tasks

import (
	"context"
	"errors"
	"sort"
	"time"

	"gv-api/internal/database/tasksdb"
	"gv-api/internal/history"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")
var ErrActiveTimeEntryExists = errors.New("an active time entry already exists")
var ErrCircularDependency = errors.New("circular task dependency")

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time, taskType string, recurrence *int32, priority int32) (TaskResponse, error)
	CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error)
	CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error)
	UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error)
	UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error)
	UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error)
	UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error)
	ListProjectsFast(ctx context.Context) ([]ProjectFastResponse, error)
	ListTasksFast(ctx context.Context) ([]TaskFastResponse, error)
	GetRootProjects(ctx context.Context) ([]ProjectResponse, error)
	GetActiveProjects(ctx context.Context) ([]ActiveProject, error)
	GetUnfinishedTasks(ctx context.Context, minPriority *int32) ([]UnfinishedTask, error)
	GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error)
	GetTask(ctx context.Context, id int32) (TaskFullResponse, error)
	GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
	GetTasksByDueDate(ctx context.Context, minPriority *int32) ([]TaskByDueDateResponse, error)
	FinishDescendantProjects(ctx context.Context, projectID int32) error
	FinishTasksByProjectTree(ctx context.Context, projectID int32) error
	DeleteProject(ctx context.Context, id int32) error
	DeleteTask(ctx context.Context, id int32) error
	DeleteTodo(ctx context.Context, id int32) error
	DeleteTimeEntry(ctx context.Context, id int32) error
	GetActiveTimeEntry(ctx context.Context) (ActiveTimeEntryResponse, error)
	GetTimeEntrySummary(ctx context.Context, todayStart, weekStart time.Time) (TimeEntrySummaryResponse, error)
	GetTimeEntryHistory(ctx context.Context, frequency, timezone string, startAt, endAt time.Time) ([]history.Point, error)
	ReplaceTaskDependencies(ctx context.Context, taskID int32, dependsOn []int32) error
	ReplaceTaskBlocks(ctx context.Context, taskID int32, blocks []int32) error
	GetTaskDependencies(ctx context.Context, taskID int32) ([]TaskDepRef, []TaskDepRef, bool, error)
	GetTimeEntriesByDateRange(ctx context.Context, startTime, endTime time.Time) ([]TimeEntryWithTaskResponse, error)
}

type PostgresRepository struct {
	q tasksdb.Querier
}

func NewRepository(q tasksdb.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time {
	if ts.Valid {
		t := ts.Time
		return &t
	}
	return nil
}

func pgDateToPtr(d pgtype.Date) *time.Time {
	if d.Valid {
		t := d.Time
		return &t
	}
	return nil
}

// anyDateToPtr handles a date column scanned into interface{} (sqlc emits
// this for date expressions whose nullability it cannot infer through CTEs).
// pgx returns either time.Time or nil for a date scanned into interface{}.
func anyDateToPtr(v interface{}) *time.Time {
	if v == nil {
		return nil
	}
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

func (r *PostgresRepository) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateProject(ctx, tasksdb.CreateProjectParams{
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
		ParentID:    parentID,
	})
	if err != nil {
		return ProjectResponse{}, err
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgDateToPtr(row.DueAt),
		ParentID:    row.ParentID,
	}, nil
}

func (r *PostgresRepository) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time, taskType string, recurrence *int32, priority int32) (TaskResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateTask(ctx, tasksdb.CreateTaskParams{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
		TaskType:    taskType,
		Recurrence:  recurrence,
		Priority:    priority,
	})
	if err != nil {
		return TaskResponse{}, err
	}

	return TaskResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgDateToPtr(row.DueAt),
		TaskType:    row.TaskType,
		Recurrence:  row.Recurrence,
		Priority:    row.Priority,
	}, nil
}

func (r *PostgresRepository) CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error) {
	row, err := r.q.CreateTodo(ctx, tasksdb.CreateTodoParams{
		TaskID: taskID,
		Name:   name,
	})
	if err != nil {
		return TodoResponse{}, err
	}

	return TodoResponse{
		ID:     row.ID,
		TaskID: row.TaskID,
		Name:   row.Name,
	}, nil
}

func (r *PostgresRepository) UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
	params := tasksdb.UpdateProjectParams{ID: req.ID}
	if req.Name != nil {
		params.SetName = true
		params.Name = *req.Name
	}
	if req.Description != nil {
		params.SetDescription = true
		params.Description = *req.Description
	}
	if req.DueAt.Set {
		if req.DueAt.Value == nil {
			params.ClearDueAt = true
		} else {
			params.SetDueAt = true
			params.DueAt = *req.DueAt.Value
		}
	}
	if req.ParentID != nil {
		params.SetParentID = true
		params.ParentID = *req.ParentID
	}
	if req.StartedAt != nil {
		params.SetStartedAt = true
		params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt, Valid: true}
	}
	if req.FinishedAt != nil {
		params.SetFinishedAt = true
		params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt, Valid: true}
	}

	row, err := r.q.UpdateProject(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectResponse{}, ErrNotFound
		}
		return ProjectResponse{}, err
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgDateToPtr(row.DueAt),
		ParentID:    row.ParentID,
		StartedAt:   pgTimestamptzToPtr(row.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(row.FinishedAt),
	}, nil
}

func (r *PostgresRepository) UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
	params := tasksdb.UpdateTaskParams{ID: req.ID}
	if req.Name != nil {
		params.SetName = true
		params.Name = *req.Name
	}
	if req.Description != nil {
		params.SetDescription = true
		params.Description = *req.Description
	}
	if req.DueAt.Set {
		if req.DueAt.Value != nil {
			params.SetDueAt = true
			params.DueAt = *req.DueAt.Value
		} else {
			params.ClearDueAt = true
		}
	}
	if req.ProjectID != nil {
		params.SetProjectID = true
		params.ProjectID = *req.ProjectID
	}
	if req.StartedAt != nil {
		params.SetStartedAt = true
		params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt, Valid: true}
	}
	if req.FinishedAt != nil {
		params.SetFinishedAt = true
		params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt, Valid: true}
	}
	if req.TaskType != nil {
		params.SetTaskType = true
		params.TaskType = *req.TaskType
		if *req.TaskType != "recurring" {
			params.ClearRecurrence = true
		}
	}
	if req.Recurrence != nil {
		params.SetRecurrence = true
		params.Recurrence = *req.Recurrence
	}
	if req.Priority != nil {
		params.SetPriority = true
		params.Priority = *req.Priority
	}

	row, err := r.q.UpdateTask(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskResponse{}, ErrNotFound
		}
		return TaskResponse{}, err
	}

	return TaskResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgDateToPtr(row.DueAt),
		StartedAt:   pgTimestamptzToPtr(row.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(row.FinishedAt),
		TaskType:    row.TaskType,
		Recurrence:  row.Recurrence,
		Priority:    row.Priority,
	}, nil
}

func (r *PostgresRepository) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	params := tasksdb.UpdateTodoParams{ID: req.ID}
	if req.TaskID != nil {
		params.SetTaskID = true
		params.TaskID = *req.TaskID
	}
	if req.Name != nil {
		params.SetName = true
		params.Name = *req.Name
	}
	if req.IsDone != nil {
		params.SetIsDone = true
		params.IsDone = *req.IsDone
	}

	row, err := r.q.UpdateTodo(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TodoResponse{}, ErrNotFound
		}
		return TodoResponse{}, err
	}

	return TodoResponse{
		ID:     row.ID,
		TaskID: row.TaskID,
		Name:   row.Name,
		IsDone: row.IsDone,
	}, nil
}

func (r *PostgresRepository) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	params := tasksdb.UpdateTimeEntryParams{ID: req.ID}
	if req.TaskID != nil {
		params.SetTaskID = true
		params.TaskID = *req.TaskID
	}
	if req.StartedAt != nil {
		params.SetStartedAt = true
		params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt, Valid: true}
	}
	if req.FinishedAt != nil {
		params.SetFinishedAt = true
		params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt, Valid: true}
	}
	if req.Comment != nil {
		params.SetComment = true
		params.Comment = *req.Comment
	}

	row, err := r.q.UpdateTimeEntry(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimeEntryResponse{}, ErrNotFound
		}
		return TimeEntryResponse{}, err
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: pgTimestamptzToPtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
	pgStartedAt := pgtype.Timestamptz{Time: startedAt, Valid: true}

	var pgFinishedAt pgtype.Timestamptz
	if finishedAt != nil {
		pgFinishedAt = pgtype.Timestamptz{Time: *finishedAt, Valid: true}
	}

	row, err := r.q.CreateTimeEntry(ctx, tasksdb.CreateTimeEntryParams{
		TaskID:     taskID,
		StartedAt:  pgStartedAt,
		FinishedAt: pgFinishedAt,
		Comment:    comment,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TimeEntryResponse{}, ErrActiveTimeEntryExists
		}
		return TimeEntryResponse{}, err
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: pgTimestamptzToPtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}


func (r *PostgresRepository) GetActiveProjects(ctx context.Context) ([]ActiveProject, error) {
	rows, err := r.q.GetActiveProjects(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ActiveProject, len(rows))
	for i, row := range rows {
		projects[i] = ActiveProject{
			ID:       row.ID,
			ParentID: row.ParentID,
			Name:     row.Name,
			DueAt:    pgDateToPtr(row.DueAt),
		}
	}
	return projects, nil
}

func (r *PostgresRepository) GetUnfinishedTasks(ctx context.Context, minPriority *int32) ([]UnfinishedTask, error) {
	rows, err := r.q.GetUnfinishedTasks(ctx, minPriority)
	if err != nil {
		return nil, err
	}

	tasks := make([]UnfinishedTask, len(rows))
	for i, row := range rows {
		tasks[i] = UnfinishedTask{
			ID:          row.ID,
			ProjectID:   row.ProjectID,
			Name:        row.Name,
			Description: row.Description,
			DueAt:       anyDateToPtr(row.DueAt),
			Started:     row.StartedAt.Valid,
			StartedAt:   pgTimestamptzToPtr(row.StartedAt),
			TaskType:    row.TaskType,
			Recurrence:  row.Recurrence,
			Priority:    row.Priority,
			DependsOn:   unmarshalDepRefs(row.DependsOn),
			Blocks:      unmarshalDepRefs(row.Blocks),
			Blocked:     row.Blocked,
		}
	}
	return tasks, nil
}

func (r *PostgresRepository) GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error) {
	resp, err := r.GetProjectChildren(ctx, id)
	if err != nil {
		return ProjectDetailResponse{}, err
	}
	return resp.Project, nil
}

func (r *PostgresRepository) GetTask(ctx context.Context, id int32) (TaskFullResponse, error) {
	row, err := r.q.GetTaskByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskFullResponse{}, ErrNotFound
		}
		return TaskFullResponse{}, err
	}

	return TaskFullResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		ProjectName: row.ProjectName,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgDateToPtr(row.DueAt),
		StartedAt:   pgTimestamptzToPtr(row.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(row.FinishedAt),
		TaskType:    row.TaskType,
		Recurrence:  row.Recurrence,
		Priority:    row.Priority,
		TimeSpent:   row.TimeSpent,
		DependsOn:   unmarshalDepRefs(row.DependsOn),
		Blocks:      unmarshalDepRefs(row.Blocks),
		Blocked:     row.Blocked,
		Todos:       unmarshalTodos(row.Todos, row.ID),
	}, nil
}

func (r *PostgresRepository) GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
	descendants, err := r.q.GetProjectWithDescendants(ctx, projectID)
	if err != nil {
		return ProjectChildrenResponse{}, err
	}
	if len(descendants) == 0 {
		return ProjectChildrenResponse{}, ErrNotFound
	}

	projectIDs := make([]int32, len(descendants))
	for i, d := range descendants {
		projectIDs[i] = d.ID
	}

	taskRows, err := r.q.GetTasksByProjectIDs(ctx, projectIDs)
	if err != nil {
		return ProjectChildrenResponse{}, err
	}

	projectTasks := make(map[int32][]ProjectChildNode)
	for _, row := range taskRows {
		blocked := row.Blocked
		priority := row.Priority
		taskType := row.TaskType
		node := ProjectChildNode{
			ID:          row.ID,
			Type:        "task",
			Name:        row.Name,
			Description: row.Description,
			DueAt:       pgDateToPtr(row.DueAt),
			StartedAt:   pgTimestamptzToPtr(row.StartedAt),
			FinishedAt:  pgTimestamptzToPtr(row.FinishedAt),
			TimeSpent:   row.TimeSpent,
			ProjectID:   row.ProjectID,
			TaskType:    &taskType,
			Recurrence:  row.Recurrence,
			Priority:    &priority,
			DependsOn:   unmarshalDepRefs(row.DependsOn),
			Blocks:      unmarshalDepRefs(row.Blocks),
			Blocked:     &blocked,
			Todos:       unmarshalTodos(row.Todos, row.ID),
		}
		if row.ProjectID != nil {
			projectTasks[*row.ProjectID] = append(projectTasks[*row.ProjectID], node)
		}
	}

	root := descendants[0]
	project := ProjectDetailResponse{
		ID:          root.ID,
		ParentID:    root.ParentID,
		Name:        root.Name,
		Description: root.Description,
		DueAt:       pgDateToPtr(root.DueAt),
		StartedAt:   pgTimestamptzToPtr(root.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(root.FinishedAt),
		TimeSpent:   root.TimeSpent,
	}

	var children []ProjectChildNode
	for _, d := range descendants[1:] {
		if d.ParentID != nil && *d.ParentID == projectID {
			children = append(children, ProjectChildNode{
				ID:          d.ID,
				Type:        "project",
				Name:        d.Name,
				Description: d.Description,
				DueAt:       pgDateToPtr(d.DueAt),
				StartedAt:   pgTimestamptzToPtr(d.StartedAt),
				FinishedAt:  pgTimestamptzToPtr(d.FinishedAt),
				TimeSpent:   d.TimeSpent,
				ParentID:    d.ParentID,
			})
		}
	}
	children = append(children, topoSortByDeps(projectTasks[projectID])...)

	if children == nil {
		children = []ProjectChildNode{}
	}

	return ProjectChildrenResponse{
		Project:  project,
		Children: children,
	}, nil
}

// topoSortByDeps reorders a project's task list so that any task whose
// DependsOn references another task in the same list comes after that
// dependency. The pre-existing relative order is preserved as the tiebreaker
// (Kahn's algorithm with a min-heap keyed by original index).
func topoSortByDeps(nodes []ProjectChildNode) []ProjectChildNode {
	n := len(nodes)
	if n <= 1 {
		return nodes
	}
	idx := make(map[int32]int, n)
	for i, t := range nodes {
		idx[t.ID] = i
	}
	inDeg := make([]int, n)
	blockedBy := make([][]int, n)
	for i, t := range nodes {
		for _, d := range t.DependsOn {
			j, ok := idx[d.ID]
			if !ok {
				continue
			}
			inDeg[i]++
			blockedBy[j] = append(blockedBy[j], i)
		}
	}
	ready := make([]int, 0, n)
	for i, deg := range inDeg {
		if deg == 0 {
			ready = append(ready, i)
		}
	}
	sort.Ints(ready)
	out := make([]ProjectChildNode, 0, n)
	for len(ready) > 0 {
		cur := ready[0]
		ready = ready[1:]
		out = append(out, nodes[cur])
		for _, j := range blockedBy[cur] {
			inDeg[j]--
			if inDeg[j] == 0 {
				pos := sort.SearchInts(ready, j)
				ready = append(ready, 0)
				copy(ready[pos+1:], ready[pos:])
				ready[pos] = j
			}
		}
	}
	if len(out) != n {
		return nodes
	}
	return out
}

func (r *PostgresRepository) GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
	rows, err := r.q.GetTimeEntriesByTaskID(ctx, taskID)
	if err != nil {
		return TaskTimeEntriesResponse{}, err
	}

	if len(rows) == 0 {
		return TaskTimeEntriesResponse{}, ErrNotFound
	}

	first := rows[0]

	var entries []TimeEntryResponse
	for _, row := range rows {
		if row.TimeEntryID == nil {
			continue
		}
		entries = append(entries, TimeEntryResponse{
			ID:         *row.TimeEntryID,
			TaskID:     row.TaskID,
			StartedAt:  row.EntryStartedAt.Time,
			FinishedAt: pgTimestamptzToPtr(row.EntryFinishedAt),
			Comment:    row.Comment,
		})
	}

	if entries == nil {
		entries = []TimeEntryResponse{}
	}

	return TaskTimeEntriesResponse{
		Task: TaskDetailResponse{
			ID:          first.TaskID,
			ProjectID:   first.ProjectID,
			Name:        first.Name,
			Description: first.Description,
			DueAt:       pgDateToPtr(first.DueAt),
			StartedAt:   pgTimestamptzToPtr(first.TaskStartedAt),
			FinishedAt:  pgTimestamptzToPtr(first.TaskFinishedAt),
			TaskType:    first.TaskType,
			Recurrence:  first.Recurrence,
			Priority:    first.Priority,
			TimeSpent:   first.TimeSpent,
			DependsOn: unmarshalDepRefs(first.DependsOn),
			Blocks:    unmarshalDepRefs(first.Blocks),
			Blocked:   first.Blocked,
		},
		TimeEntries: entries,
	}, nil
}

func (r *PostgresRepository) GetTasksByDueDate(ctx context.Context, minPriority *int32) ([]TaskByDueDateResponse, error) {
	rows, err := r.q.GetTasksByDueDate(ctx, minPriority)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskByDueDateResponse, len(rows))
	for i, row := range rows {
		tasks[i] = TaskByDueDateResponse{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			DueAt:        anyDateToPtr(row.DueAt),
			StartedAt:    pgTimestamptzToPtr(row.StartedAt),
			TaskType:     row.TaskType,
			Recurrence:   row.Recurrence,
			Priority:     row.Priority,
			TimeSpent:    row.TimeSpent,
			ProjectID:    row.ProjectID,
			ProjectName:  row.ProjectName,
			ProjectDueAt: pgDateToPtr(row.ProjectDueAt),
			DependsOn:    unmarshalDepRefs(row.DependsOn),
			Blocks:       unmarshalDepRefs(row.Blocks),
			Blocked:      row.Blocked,
		}
	}
	return tasks, nil
}

func (r *PostgresRepository) FinishDescendantProjects(ctx context.Context, projectID int32) error {
	return r.q.FinishDescendantProjects(ctx, projectID)
}

func (r *PostgresRepository) FinishTasksByProjectTree(ctx context.Context, projectID int32) error {
	return r.q.FinishTasksByProjectTree(ctx, projectID)
}

func (r *PostgresRepository) DeleteProject(ctx context.Context, id int32) error {
	return r.q.DeleteProject(ctx, id)
}

func (r *PostgresRepository) DeleteTask(ctx context.Context, id int32) error {
	return r.q.DeleteTask(ctx, id)
}

func (r *PostgresRepository) DeleteTodo(ctx context.Context, id int32) error {
	return r.q.DeleteTodo(ctx, id)
}

func (r *PostgresRepository) DeleteTimeEntry(ctx context.Context, id int32) error {
	return r.q.DeleteTimeEntry(ctx, id)
}

func (r *PostgresRepository) GetActiveTimeEntry(ctx context.Context) (ActiveTimeEntryResponse, error) {
	row, err := r.q.GetActiveTimeEntry(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActiveTimeEntryResponse{}, ErrNotFound
		}
		return ActiveTimeEntryResponse{}, err
	}

	return ActiveTimeEntryResponse{
		ID:          row.ID,
		TaskID:      row.TaskID,
		StartedAt:   row.StartedAt.Time,
		FinishedAt:  pgTimestamptzToPtr(row.FinishedAt),
		Comment:     row.Comment,
		TaskName:    row.TaskName,
		TaskType:    row.TaskType,
		Recurrence:  row.Recurrence,
		Priority:    row.Priority,
		ProjectName: row.ProjectName,
	}, nil
}

func (r *PostgresRepository) GetTimeEntrySummary(ctx context.Context, todayStart, weekStart time.Time) (TimeEntrySummaryResponse, error) {
	row, err := r.q.GetTimeEntrySummary(ctx, tasksdb.GetTimeEntrySummaryParams{
		TodayStart: pgtype.Timestamptz{Time: todayStart, Valid: true},
		WeekStart:  pgtype.Timestamptz{Time: weekStart, Valid: true},
	})
	if err != nil {
		return TimeEntrySummaryResponse{}, err
	}

	return TimeEntrySummaryResponse{
		Today: row.Today,
		Week:  row.Week,
	}, nil
}

func (r *PostgresRepository) GetTimeEntryHistory(ctx context.Context, frequency, timezone string, startAt, endAt time.Time) ([]history.Point, error) {
	rows, err := r.q.GetTimeEntryHistory(ctx, tasksdb.GetTimeEntryHistoryParams{
		Frequency: frequency,
		Timezone:  timezone,
		StartAt:   startAt,
		EndAt:     endAt,
	})
	if err != nil {
		return nil, err
	}

	results := make([]history.Point, len(rows))
	for i, row := range rows {
		results[i] = history.Point{
			Date:  row.Date.Format("2006-01-02"),
			Value: row.Value,
		}
	}
	return results, nil
}

func (r *PostgresRepository) GetTimeEntriesByDateRange(ctx context.Context, startTime, endTime time.Time) ([]TimeEntryWithTaskResponse, error) {
	rows, err := r.q.GetTimeEntriesByDateRange(ctx, tasksdb.GetTimeEntriesByDateRangeParams{
		StartTime: pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:   pgtype.Timestamptz{Time: endTime, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	entries := make([]TimeEntryWithTaskResponse, len(rows))
	for i, row := range rows {
		entries[i] = TimeEntryWithTaskResponse{
			ID:             row.ID,
			TaskID:         row.TaskID,
			TaskName:       row.TaskName,
			TaskType:       row.TaskType,
			Recurrence:     row.Recurrence,
			Priority:       row.Priority,
			ProjectID:      row.ProjectID,
			ProjectName:    row.ProjectName,
			StartedAt:      row.StartedAt.Time,
			FinishedAt:     pgTimestamptzToPtr(row.FinishedAt),
			Comment:        row.Comment,
			TaskFinishedAt: pgTimestamptzToPtr(row.TaskFinishedAt),
			TimeSpent:      row.TimeSpent,
		}
	}

	return entries, nil
}

func (r *PostgresRepository) ReplaceTaskDependencies(ctx context.Context, taskID int32, dependsOn []int32) error {
	if len(dependsOn) > 0 {
		hasCycle, err := r.q.TaskDependencyWouldCycle(ctx, tasksdb.TaskDependencyWouldCycleParams{
			TaskID:   taskID,
			NewDeps:  dependsOn,
		})
		if err != nil {
			return err
		}
		if hasCycle {
			return ErrCircularDependency
		}
	}
	if err := r.q.DeleteRemovedTaskDependencies(ctx, tasksdb.DeleteRemovedTaskDependenciesParams{
		TaskID: taskID,
		Keep:   dependsOn,
	}); err != nil {
		return err
	}
	return r.q.UpsertTaskDependencies(ctx, tasksdb.UpsertTaskDependenciesParams{
		TaskID:    taskID,
		DependsOn: dependsOn,
	})
}

func (r *PostgresRepository) ReplaceTaskBlocks(ctx context.Context, taskID int32, blocks []int32) error {
	if len(blocks) > 0 {
		hasCycle, err := r.q.TaskBlocksWouldCycle(ctx, tasksdb.TaskBlocksWouldCycleParams{
			Blocks: blocks,
			TaskID: taskID,
		})
		if err != nil {
			return err
		}
		if hasCycle {
			return ErrCircularDependency
		}
	}
	if err := r.q.DeleteRemovedTaskBlocks(ctx, tasksdb.DeleteRemovedTaskBlocksParams{
		DependsOn: taskID,
		Keep:      blocks,
	}); err != nil {
		return err
	}
	return r.q.UpsertTaskBlocks(ctx, tasksdb.UpsertTaskBlocksParams{
		DependsOn: taskID,
		Blocks:    blocks,
	})
}

func (r *PostgresRepository) GetTaskDependencies(ctx context.Context, taskID int32) ([]TaskDepRef, []TaskDepRef, bool, error) {
	row, err := r.q.GetTaskDependencies(ctx, taskID)
	if err != nil {
		return nil, nil, false, err
	}
	return unmarshalDepRefs(row.DependsOn), unmarshalDepRefs(row.Blocks), row.Blocked, nil
}

func (r *PostgresRepository) ListTasksFast(ctx context.Context) ([]TaskFastResponse, error) {
	rows, err := r.q.ListTasksFast(ctx)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskFastResponse, len(rows))
	for i, row := range rows {
		tasks[i] = TaskFastResponse{
			ID:          row.ID,
			Name:        row.Name,
			ProjectID:   row.ProjectID,
			ProjectName: row.ProjectName,
			TaskType:    row.TaskType,
			Recurrence:  row.Recurrence,
			Priority:    row.Priority,
		}
	}

	return tasks, nil
}

func (r *PostgresRepository) ListProjectsFast(ctx context.Context) ([]ProjectFastResponse, error) {
	rows, err := r.q.ListProjectsFast(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectFastResponse, len(rows))
	for i, row := range rows {
		projects[i] = ProjectFastResponse{
			ID:   row.ID,
			Name: row.Name,
		}
	}

	return projects, nil
}

func (r *PostgresRepository) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	rows, err := r.q.GetRootProjects(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectResponse, len(rows))
	for i, row := range rows {
		projects[i] = ProjectResponse{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			DueAt:       pgDateToPtr(row.DueAt),
			ParentID:    row.ParentID,
		}
	}

	return projects, nil
}
