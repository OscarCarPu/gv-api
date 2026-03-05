package tasks

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"gv-api/internal/database/tasksdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error)
	CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error)
	CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error)
	FinishTimeEntry(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error)
	FinishTask(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error)
	FinishProject(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error)
	GetRootProjects(ctx context.Context) ([]ProjectResponse, error)
	GetActiveProjects(ctx context.Context) ([]ActiveProject, error)
	GetUnfinishedTasks(ctx context.Context) ([]UnfinishedTask, error)
	GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
}

type PostgresRepository struct {
	q tasksdb.Querier
}

func NewRepository(q tasksdb.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func pgTimestampToPtr(ts pgtype.Timestamp) *time.Time {
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

func (r *PostgresRepository) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateTask(ctx, tasksdb.CreateTaskParams{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
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

func (r *PostgresRepository) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
	pgStartedAt := pgtype.Timestamp{Time: startedAt, Valid: true}

	var pgFinishedAt pgtype.Timestamp
	if finishedAt != nil {
		pgFinishedAt = pgtype.Timestamp{Time: *finishedAt, Valid: true}
	}

	row, err := r.q.CreateTimeEntry(ctx, tasksdb.CreateTimeEntryParams{
		TaskID:     taskID,
		StartedAt:  pgStartedAt,
		FinishedAt: pgFinishedAt,
		Comment:    comment,
	})
	if err != nil {
		return TimeEntryResponse{}, err
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: pgTimestampToPtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) FinishTimeEntry(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishTimeEntry(ctx, tasksdb.FinishTimeEntryParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
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
		FinishedAt: pgTimestampToPtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) FinishTask(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishTask(ctx, tasksdb.FinishTaskParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
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
		StartedAt:   pgTimestampToPtr(row.StartedAt),
		FinishedAt:  pgTimestampToPtr(row.FinishedAt),
	}, nil
}

func (r *PostgresRepository) FinishProject(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishProject(ctx, tasksdb.FinishProjectParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
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
		StartedAt:   pgTimestampToPtr(row.StartedAt),
		FinishedAt:  pgTimestampToPtr(row.FinishedAt),
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
			ID:        row.ID,
			ProjectID: row.ProjectID,
			Name:      row.Name,
			Started:   row.StartedAt.Valid,
		}
	}
	return tasks, nil
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

		node := ProjectChildNode{
			ID:          tw.row.ID,
			Type:        "task",
			Name:        tw.row.Name,
			Description: tw.row.Description,
			DueAt:       pgDateToPtr(tw.row.DueAt),
			StartedAt:   pgTimestampToPtr(tw.row.StartedAt),
			FinishedAt:  pgTimestampToPtr(tw.row.FinishedAt),
			TimeSpent:   tw.row.TimeSpent,
			ProjectID:   tw.row.ProjectID,
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
		StartedAt:   pgTimestampToPtr(root.StartedAt),
		FinishedAt:  pgTimestampToPtr(root.FinishedAt),
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
				StartedAt:   pgTimestampToPtr(d.StartedAt),
				FinishedAt:  pgTimestampToPtr(d.FinishedAt),
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
			FinishedAt: pgTimestampToPtr(row.EntryFinishedAt),
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
			StartedAt:   pgTimestampToPtr(first.TaskStartedAt),
			FinishedAt:  pgTimestampToPtr(first.TaskFinishedAt),
			TimeSpent:   first.TimeSpent,
		},
		TimeEntries: entries,
	}, nil
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
