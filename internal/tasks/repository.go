package tasks

import (
	"context"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/history"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time, taskType string, recurrence *int32, priority int32, estimateHours *decimal.Decimal) (TaskResponse, error)
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
	pool *pgxpool.Pool
	q    *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: gvdb.New(pool)}
}

func (r *PostgresRepository) withTx(ctx context.Context, fn func(*gvdb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) deleteByID(ctx context.Context, table string, id int32) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
