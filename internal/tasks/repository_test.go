package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/tasksdb"
	"gv-api/internal/tasks"
	testutil "gv-api/internal/testutils"

	"github.com/stretchr/testify/require"
)

func NewRepo(t *testing.T) *tasks.PostgresRepository {
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "task_dependencies", "time_entries", "todos", "tasks", "projects")
	return tasks.NewRepository(tasksdb.New(pool))
}

func TestIntegration_ActiveTimeEntryUnique(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	task, err := repo.CreateTask(ctx, nil, "t", nil, nil, "standard", nil, 4)
	require.NoError(t, err)

	_, err = repo.CreateTimeEntry(ctx, task.ID, time.Now(), nil, nil)
	require.NoError(t, err)

	_, err = repo.CreateTimeEntry(ctx, task.ID, time.Now(), nil, nil)
	require.True(t, errors.Is(err, tasks.ErrActiveTimeEntryExists), "second open time entry should return ErrActiveTimeEntryExists, got %v", err)
}

func TestIntegration_FinishProjectTreeCascades(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	root, err := repo.CreateProject(ctx, "root", nil, nil, nil)
	require.NoError(t, err)
	mid, err := repo.CreateProject(ctx, "mid", nil, nil, &root.ID)
	require.NoError(t, err)
	leaf, err := repo.CreateProject(ctx, "leaf", nil, nil, &mid.ID)
	require.NoError(t, err)

	leafTask, err := repo.CreateTask(ctx, &leaf.ID, "t", nil, nil, "standard", nil, 4)
	require.NoError(t, err)

	require.NoError(t, repo.FinishDescendantProjects(ctx, root.ID))
	require.NoError(t, repo.FinishTasksByProjectTree(ctx, root.ID))

	got, err := repo.GetTask(ctx, leafTask.ID)
	require.NoError(t, err)
	require.NotNil(t, got.FinishedAt, "leaf task should be finished by cascade")

	gotProj, err := repo.GetProject(ctx, leaf.ID)
	require.NoError(t, err)
	require.NotNil(t, gotProj.FinishedAt, "leaf project should be finished by cascade")
}

func TestIntegration_ReplaceTaskDependencies(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	mk := func(name string) int32 {
		task, err := repo.CreateTask(ctx, nil, name, nil, nil, "standard", nil, 4)
		require.NoError(t, err)
		return task.ID
	}
	a, b, c, d := mk("a"), mk("b"), mk("c"), mk("d")

	depIDs := func(t *testing.T, id int32) []int32 {
		t.Helper()
		deps, _, _, err := repo.GetTaskDependencies(ctx, id)
		require.NoError(t, err)
		out := make([]int32, len(deps))
		for i, dep := range deps {
			out[i] = dep.ID
		}
		return out
	}

	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a, []int32{b, c}))
	require.ElementsMatch(t, []int32{b, c}, depIDs(t, a))

	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a, []int32{b, c, d}))
	require.ElementsMatch(t, []int32{b, c, d}, depIDs(t, a))

	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a, []int32{d}))
	require.ElementsMatch(t, []int32{d}, depIDs(t, a),
		"replace should remove old deps not in the new set")

	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a, []int32{}))
	require.Empty(t, depIDs(t, a))
}

func TestIntegration_CircularTaskDependenciesRejected(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	mk := func(name string) int32 {
		task, err := repo.CreateTask(ctx, nil, name, nil, nil, "standard", nil, 4)
		require.NoError(t, err)
		return task.ID
	}
	a, b := mk("a"), mk("b")

	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a, []int32{b}))
	require.Error(t, repo.ReplaceTaskDependencies(ctx, b, []int32{a}),
		"creating a cycle (A→B, B→A) must be rejected")
}
