package tasks

import (
	"cmp"
	"context"
	"errors"
	"slices"
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
	GetUnfinishedTasks(ctx context.Context) ([]UnfinishedTask, error)
	GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error)
	GetTask(ctx context.Context, id int32) (TaskFullResponse, error)
	GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
	GetTasksByDueDate(ctx context.Context) ([]TaskByDueDateResponse, error)
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

func (r *PostgresRepository) GetUnfinishedTasks(ctx context.Context) ([]UnfinishedTask, error) {
	rows, err := r.q.GetUnfinishedTasks(ctx)
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
			DueAt:       pgDateToPtr(row.DueAt),
			Started:     row.StartedAt.Valid,
			StartedAt:   pgTimestamptzToPtr(row.StartedAt),
			TaskType:    row.TaskType,
			Recurrence:  row.Recurrence,
			Priority:    row.Priority,
			DependsOn:   unmarshalDepRefs(row.DependsOn),
			Blocks: unmarshalDepRefs(row.Blocks),
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
	rows, err := r.q.GetTaskByID(ctx, id)
	if err != nil {
		return TaskFullResponse{}, err
	}
	if len(rows) == 0 {
		return TaskFullResponse{}, ErrNotFound
	}

	first := rows[0]
	var todos []TodoResponse
	for _, row := range rows {
		if row.TodoID != nil {
			todos = append(todos, TodoResponse{
				ID:     *row.TodoID,
				TaskID: first.ID,
				Name:   *row.TodoName,
				IsDone: *row.TodoIsDone,
			})
		}
	}
	if todos == nil {
		todos = []TodoResponse{}
	}

	return TaskFullResponse{
		ID:          first.ID,
		ProjectID:   first.ProjectID,
		Name:        first.Name,
		Description: first.Description,
		DueAt:       pgDateToPtr(first.DueAt),
		StartedAt:   pgTimestamptzToPtr(first.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(first.FinishedAt),
		TaskType:    first.TaskType,
		Recurrence:  first.Recurrence,
		Priority:    first.Priority,
		TimeSpent:   first.TimeSpent,
		DependsOn: unmarshalDepRefs(first.DependsOn),
		Blocks:    unmarshalDepRefs(first.Blocks),
		Blocked:   first.Blocked,
		Todos:     todos,
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

	// Collect all project IDs for task query
	projectIDs := make([]int32, len(descendants))
	for i, d := range descendants {
		projectIDs[i] = d.ID
	}

	taskRows, err := r.q.GetTasksByProjectIDs(ctx, projectIDs)
	if err != nil {
		return ProjectChildrenResponse{}, err
	}

	// Group task rows by task ID (multiple rows per task due to LEFT JOIN todos)
	type taskWithTodos struct {
		row   tasksdb.GetTasksByProjectIDsRow
		todos []TodoResponse
	}
	taskMap := make(map[int32]*taskWithTodos)
	var taskOrder []int32
	for _, row := range taskRows {
		tw, exists := taskMap[row.ID]
		if !exists {
			tw = &taskWithTodos{row: row}
			taskMap[row.ID] = tw
			taskOrder = append(taskOrder, row.ID)
		}
		if row.TodoID != nil {
			tw.todos = append(tw.todos, TodoResponse{
				ID:     *row.TodoID,
				TaskID: row.ID,
				Name:   *row.TodoName,
				IsDone: *row.TodoIsDone,
			})
		}
	}

	// Build per-project direct task time_spent
	projectTaskTime := make(map[int32]int64)
	// Also build tasks grouped by project
	projectTasks := make(map[int32][]ProjectChildNode)
	for _, id := range taskOrder {
		tw := taskMap[id]
		if tw.row.ProjectID != nil {
			projectTaskTime[*tw.row.ProjectID] += tw.row.TimeSpent
		}

		blocked := tw.row.Blocked
		priority := tw.row.Priority
		node := ProjectChildNode{
			ID:          tw.row.ID,
			Type:        "task",
			Name:        tw.row.Name,
			Description: tw.row.Description,
			DueAt:       pgDateToPtr(tw.row.DueAt),
			StartedAt:   pgTimestamptzToPtr(tw.row.StartedAt),
			FinishedAt:  pgTimestamptzToPtr(tw.row.FinishedAt),
			TimeSpent:   tw.row.TimeSpent,
			ProjectID:   tw.row.ProjectID,
			TaskType:    &tw.row.TaskType,
			Recurrence:  tw.row.Recurrence,
			Priority:    &priority,
			DependsOn:   unmarshalDepRefs(tw.row.DependsOn),
			Blocks:      unmarshalDepRefs(tw.row.Blocks),
			Blocked:     &blocked,
			Todos:       tw.todos,
		}
		if tw.row.ProjectID != nil {
			projectTasks[*tw.row.ProjectID] = append(projectTasks[*tw.row.ProjectID], node)
		}
	}

	// Build descendant map indexed by ID, with depth for bottom-up accumulation
	type projectInfo struct {
		row      tasksdb.GetProjectWithDescendantsRow
		parentID *int32
		depth    int32
	}
	projectInfoMap := make(map[int32]*projectInfo, len(descendants))
	for _, d := range descendants {
		projectInfoMap[d.ID] = &projectInfo{row: d, parentID: d.ParentID, depth: d.Depth}
	}

	// Sort descendants by depth descending for bottom-up time accumulation
	sorted := make([]tasksdb.GetProjectWithDescendantsRow, len(descendants))
	copy(sorted, descendants)
	slices.SortFunc(sorted, func(a, b tasksdb.GetProjectWithDescendantsRow) int {
		return cmp.Compare(b.Depth, a.Depth)
	})

	// Accumulate time_spent bottom-up
	projectTimeSpent := make(map[int32]int64)
	for _, d := range sorted {
		projectTimeSpent[d.ID] += projectTaskTime[d.ID]
		if d.ParentID != nil {
			if _, ok := projectInfoMap[*d.ParentID]; ok {
				projectTimeSpent[*d.ParentID] += projectTimeSpent[d.ID]
			}
		}
	}

	// Build root project response
	root := descendants[0]
	project := ProjectDetailResponse{
		ID:          root.ID,
		ParentID:    root.ParentID,
		Name:        root.Name,
		Description: root.Description,
		DueAt:       pgDateToPtr(root.DueAt),
		StartedAt:   pgTimestamptzToPtr(root.StartedAt),
		FinishedAt:  pgTimestamptzToPtr(root.FinishedAt),
		TimeSpent:   projectTimeSpent[root.ID],
	}

	// Build children: direct sub-projects first, then tasks
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
				TimeSpent:   projectTimeSpent[d.ID],
				ParentID:    d.ParentID,
			})
		}
	}
	children = append(children, projectTasks[projectID]...)

	if children == nil {
		children = []ProjectChildNode{}
	}

	return ProjectChildrenResponse{
		Project:  project,
		Children: children,
	}, nil
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

func (r *PostgresRepository) GetTasksByDueDate(ctx context.Context) ([]TaskByDueDateResponse, error) {
	rows, err := r.q.GetTasksByDueDate(ctx)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskByDueDateResponse, len(rows))
	for i, row := range rows {
		tasks[i] = TaskByDueDateResponse{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			DueAt:        pgDateToPtr(row.DueAt),
			StartedAt:    pgTimestamptzToPtr(row.StartedAt),
			TaskType:     row.TaskType,
			Recurrence:   row.Recurrence,
			Priority:     row.Priority,
			TimeSpent:    row.TimeSpent,
			ProjectID:    row.ProjectID,
			ProjectName:  row.ProjectName,
			ProjectDueAt: pgDateToPtr(row.ProjectDueAt),
			DependsOn:    unmarshalDepRefs(row.DependsOn),
			Blocks:  unmarshalDepRefs(row.Blocks),
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
