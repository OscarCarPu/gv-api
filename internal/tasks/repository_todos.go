package tasks

import (
	"context"
	"errors"

	"gv-api/internal/database/gvdb"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error) {
	row, err := r.q.CreateTodo(ctx, gvdb.CreateTodoParams{
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

func (r *PostgresRepository) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	params := gvdb.UpdateTodoParams{ID: req.ID}
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

func (r *PostgresRepository) DeleteTodo(ctx context.Context, id int32) error {
	return r.deleteByID(ctx, "todos", id)
}
