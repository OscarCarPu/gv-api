package tasks

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/database/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

func (r *PostgresRepository) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time, taskType string, recurrence *int32, priority int32, estimateHours *decimal.Decimal) (TaskResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateTask(ctx, gvdb.CreateTaskParams{
		ProjectID:     projectID,
		Name:          name,
		Description:   description,
		DueAt:         pgDueAt,
		TaskType:      taskType,
		Recurrence:    recurrence,
		Priority:      priority,
		EstimateHours: estimateHours,
	})
	if err != nil {
		return TaskResponse{}, err
	}

	return TaskResponse{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		Name:          row.Name,
		Description:   row.Description,
		DueAt:         pgconv.DatePtr(row.DueAt),
		TaskType:      row.TaskType,
		Recurrence:    row.Recurrence,
		Priority:      row.Priority,
		EstimateHours: row.EstimateHours,
	}, nil
}

func (r *PostgresRepository) UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
	params := gvdb.UpdateTaskParams{ID: req.ID}
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
	if req.StartedAt.Set {
		if req.StartedAt.Value == nil {
			params.ClearStartedAt = true
		} else {
			params.SetStartedAt = true
			params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt.Value, Valid: true}
		}
	}
	if req.FinishedAt.Set {
		if req.FinishedAt.Value == nil {
			params.ClearFinishedAt = true
		} else {
			params.SetFinishedAt = true
			params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt.Value, Valid: true}
		}
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
	if req.EstimateHours.Set {
		if req.EstimateHours.Value == nil {
			params.ClearEstimateHours = true
		} else {
			params.SetEstimateHours = true
			params.EstimateHours = *req.EstimateHours.Value
		}
	}

	row, err := r.q.UpdateTask(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskResponse{}, ErrNotFound
		}
		return TaskResponse{}, err
	}

	return TaskResponse{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		Name:          row.Name,
		Description:   row.Description,
		DueAt:         pgconv.DatePtr(row.DueAt),
		StartedAt:     pgconv.TimePtr(row.StartedAt),
		FinishedAt:    pgconv.TimePtr(row.FinishedAt),
		TaskType:      row.TaskType,
		Recurrence:    row.Recurrence,
		Priority:      row.Priority,
		EstimateHours: row.EstimateHours,
	}, nil
}

func (r *PostgresRepository) GetUnfinishedTasks(ctx context.Context, minPriority *int32) ([]UnfinishedTask, error) {
	rows, err := r.q.GetUnfinishedTasks(ctx, minPriority)
	if err != nil {
		return nil, err
	}

	tasks := make([]UnfinishedTask, len(rows))
	for i, row := range rows {
		dependsOn, err := unmarshalDepRefs(row.DependsOn)
		if err != nil {
			return nil, err
		}
		blocks, err := unmarshalDepRefs(row.Blocks)
		if err != nil {
			return nil, err
		}
		tasks[i] = UnfinishedTask{
			ID:          row.ID,
			ProjectID:   row.ProjectID,
			Name:        row.Name,
			Description: row.Description,
			DueAt:       pgconv.AnyDatePtr(row.DueAt),
			Started:     row.StartedAt.Valid,
			StartedAt:   pgconv.TimePtr(row.StartedAt),
			TaskType:    row.TaskType,
			Recurrence:  row.Recurrence,
			Priority:    row.Priority,
			DependsOn:   dependsOn,
			Blocks:      blocks,
			Blocked:     row.Blocked,
		}
	}
	return tasks, nil
}

func (r *PostgresRepository) GetTask(ctx context.Context, id int32) (TaskFullResponse, error) {
	row, err := r.q.GetTaskByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskFullResponse{}, ErrNotFound
		}
		return TaskFullResponse{}, err
	}

	dependsOn, err := unmarshalDepRefs(row.DependsOn)
	if err != nil {
		return TaskFullResponse{}, err
	}
	blocks, err := unmarshalDepRefs(row.Blocks)
	if err != nil {
		return TaskFullResponse{}, err
	}
	return TaskFullResponse{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		ProjectName:   row.ProjectName,
		Name:          row.Name,
		Description:   row.Description,
		DueAt:         pgconv.DatePtr(row.DueAt),
		StartedAt:     pgconv.TimePtr(row.StartedAt),
		FinishedAt:    pgconv.TimePtr(row.FinishedAt),
		TaskType:      row.TaskType,
		Recurrence:    row.Recurrence,
		Priority:      row.Priority,
		EstimateHours: row.EstimateHours,
		TimeSpent:     row.TimeSpent,
		DependsOn:     dependsOn,
		Blocks:        blocks,
		Blocked:       row.Blocked,
		Todos:         unmarshalTodos(row.Todos, row.ID),
	}, nil
}

func (r *PostgresRepository) GetTasksByDueDate(ctx context.Context, minPriority *int32) ([]TaskByDueDateResponse, error) {
	rows, err := r.q.GetTasksByDueDate(ctx, minPriority)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskByDueDateResponse, len(rows))
	for i, row := range rows {
		dependsOn, err := unmarshalDepRefs(row.DependsOn)
		if err != nil {
			return nil, err
		}
		blocks, err := unmarshalDepRefs(row.Blocks)
		if err != nil {
			return nil, err
		}
		tasks[i] = TaskByDueDateResponse{
			ID:            row.ID,
			Name:          row.Name,
			Description:   row.Description,
			DueAt:         pgconv.AnyDatePtr(row.DueAt),
			StartedAt:     pgconv.TimePtr(row.StartedAt),
			TaskType:      row.TaskType,
			Recurrence:    row.Recurrence,
			Priority:      row.Priority,
			TimeSpent:     row.TimeSpent,
			EstimateHours: row.EstimateHours,
			ProjectID:     row.ProjectID,
			ProjectName:   row.ProjectName,
			ProjectDueAt:  pgconv.DatePtr(row.ProjectDueAt),
			DependsOn:     dependsOn,
			Blocks:        blocks,
			Blocked:       row.Blocked,
		}
	}
	return tasks, nil
}

func (r *PostgresRepository) FinishTasksByProjectTree(ctx context.Context, projectID int32) error {
	return r.q.FinishTasksByProjectTree(ctx, projectID)
}

func (r *PostgresRepository) DeleteTask(ctx context.Context, id int32) error {
	return r.deleteByID(ctx, "tasks", id)
}

func (r *PostgresRepository) ReplaceTaskDependencies(ctx context.Context, taskID int32, dependsOn []int32) error {
	return r.withTx(ctx, func(q *gvdb.Queries) error {
		if len(dependsOn) > 0 {
			hasCycle, err := q.TaskDependencyWouldCycle(ctx, gvdb.TaskDependencyWouldCycleParams{
				TaskID:  taskID,
				NewDeps: dependsOn,
			})
			if err != nil {
				return err
			}
			if hasCycle {
				return ErrCircularDependency
			}
		}
		if err := q.DeleteRemovedTaskDependencies(ctx, gvdb.DeleteRemovedTaskDependenciesParams{
			TaskID: taskID,
			Keep:   dependsOn,
		}); err != nil {
			return err
		}
		return q.UpsertTaskDependencies(ctx, gvdb.UpsertTaskDependenciesParams{
			TaskID:    taskID,
			DependsOn: dependsOn,
		})
	})
}

func (r *PostgresRepository) ReplaceTaskBlocks(ctx context.Context, taskID int32, blocks []int32) error {
	return r.withTx(ctx, func(q *gvdb.Queries) error {
		if len(blocks) > 0 {
			hasCycle, err := q.TaskBlocksWouldCycle(ctx, gvdb.TaskBlocksWouldCycleParams{
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
		if err := q.DeleteRemovedTaskBlocks(ctx, gvdb.DeleteRemovedTaskBlocksParams{
			DependsOn: taskID,
			Keep:      blocks,
		}); err != nil {
			return err
		}
		return q.UpsertTaskBlocks(ctx, gvdb.UpsertTaskBlocksParams{
			DependsOn: taskID,
			Blocks:    blocks,
		})
	})
}

func (r *PostgresRepository) GetTaskDependencies(ctx context.Context, taskID int32) ([]TaskDepRef, []TaskDepRef, bool, error) {
	row, err := r.q.GetTaskDependencies(ctx, taskID)
	if err != nil {
		return nil, nil, false, err
	}
	dependsOn, err := unmarshalDepRefs(row.DependsOn)
	if err != nil {
		return nil, nil, false, err
	}
	blocks, err := unmarshalDepRefs(row.Blocks)
	if err != nil {
		return nil, nil, false, err
	}
	return dependsOn, blocks, row.Blocked, nil
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
