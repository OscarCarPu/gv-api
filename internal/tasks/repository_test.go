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

func TestIntegration_GetUnfinishedTasks_MinPriorityFilter(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	mkP := func(name string, priority int32) int32 {
		task, err := repo.CreateTask(ctx, nil, name, nil, nil, "standard", nil, priority)
		require.NoError(t, err)
		return task.ID
	}
	mkP("urgent", 1)
	mkP("normal", 3)
	mkP("low", 5)

	all, err := repo.GetUnfinishedTasks(ctx, nil)
	require.NoError(t, err)
	require.Len(t, all, 3, "nil min_priority returns all tasks")

	threshold := int32(2)
	filtered, err := repo.GetUnfinishedTasks(ctx, &threshold)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "urgent", filtered[0].Name)

	threshold = int32(3)
	filtered, err = repo.GetUnfinishedTasks(ctx, &threshold)
	require.NoError(t, err)
	require.Len(t, filtered, 2, "priority<=3 includes urgent and normal")
}

func TestIntegration_GetProjectChildren_BottomUpTimeAccumulation(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	root, err := repo.CreateProject(ctx, "root", nil, nil, nil)
	require.NoError(t, err)
	mid, err := repo.CreateProject(ctx, "mid", nil, nil, &root.ID)
	require.NoError(t, err)
	leaf, err := repo.CreateProject(ctx, "leaf", nil, nil, &mid.ID)
	require.NoError(t, err)

	rootTask, err := repo.CreateTask(ctx, &root.ID, "rootTask", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	leafTask, err := repo.CreateTask(ctx, &leaf.ID, "leafTask", nil, nil, "standard", nil, 4)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	rootEnd := now.Add(60 * time.Second)
	_, err = repo.CreateTimeEntry(ctx, rootTask.ID, now, &rootEnd, nil)
	require.NoError(t, err)
	leafEnd := now.Add(120 * time.Second)
	_, err = repo.CreateTimeEntry(ctx, leafTask.ID, now, &leafEnd, nil)
	require.NoError(t, err)

	resp, err := repo.GetProjectChildren(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, int64(180), resp.Project.TimeSpent, "root rolls up own task + descendants")

	respMid, err := repo.GetProjectChildren(ctx, mid.ID)
	require.NoError(t, err)
	require.Equal(t, int64(120), respMid.Project.TimeSpent, "mid only counts its descendants")

	respLeaf, err := repo.GetProjectChildren(ctx, leaf.ID)
	require.NoError(t, err)
	require.Equal(t, int64(120), respLeaf.Project.TimeSpent, "leaf counts its own tasks")
}

func TestIntegration_GetProjectChildren_BlocksOrderBeforeBlocked(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	proj, err := repo.CreateProject(ctx, "p", nil, nil, nil)
	require.NoError(t, err)

	// Create in alphabetical order so SQL would naturally sort a, b, c, d.
	// Then wire deps so the topological order must be: c -> a, d -> b.
	a, err := repo.CreateTask(ctx, &proj.ID, "a", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	b, err := repo.CreateTask(ctx, &proj.ID, "b", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	c, err := repo.CreateTask(ctx, &proj.ID, "c", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	d, err := repo.CreateTask(ctx, &proj.ID, "d", nil, nil, "standard", nil, 4)
	require.NoError(t, err)

	// a depends on c (c blocks a) and b depends on d (d blocks b).
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, a.ID, []int32{c.ID}))
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, b.ID, []int32{d.ID}))

	resp, err := repo.GetProjectChildren(ctx, proj.ID)
	require.NoError(t, err)

	names := make([]string, len(resp.Children))
	for i, ch := range resp.Children {
		names[i] = ch.Name
	}
	require.Equal(t, []string{"c", "a", "d", "b"}, names,
		"blocking tasks must appear before the tasks they block; alpha order is the tiebreaker")
}

func TestIntegration_GetProjectChildren_TodosAggregated(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	proj, err := repo.CreateProject(ctx, "p", nil, nil, nil)
	require.NoError(t, err)
	task, err := repo.CreateTask(ctx, &proj.ID, "t", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	_, err = repo.CreateTodo(ctx, task.ID, "first")
	require.NoError(t, err)
	_, err = repo.CreateTodo(ctx, task.ID, "second")
	require.NoError(t, err)

	resp, err := repo.GetProjectChildren(ctx, proj.ID)
	require.NoError(t, err)
	require.Len(t, resp.Children, 1)
	require.Equal(t, "task", resp.Children[0].Type)
	require.Len(t, resp.Children[0].Todos, 2)
}

func TestIntegration_GetUnfinishedTasks_HiddenAndBlocked(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	mk := func(name string) int32 {
		task, err := repo.CreateTask(ctx, nil, name, nil, nil, "standard", nil, 4)
		require.NoError(t, err)
		return task.ID
	}
	a, b, c := mk("a"), mk("b"), mk("c")
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, b, []int32{a}))
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, c, []int32{b}))

	rows, err := repo.GetUnfinishedTasks(ctx, nil)
	require.NoError(t, err)

	byID := make(map[int32]tasks.UnfinishedTask, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}

	require.Contains(t, byID, a, "a is not blocked, must appear")
	require.Contains(t, byID, b, "b is blocked but only by a (a is not blocked itself), so b is not hidden")
	require.NotContains(t, byID, c, "c is hidden — its only dep b is itself blocked")

	require.False(t, byID[a].Blocked, "a has no deps")
	require.True(t, byID[b].Blocked, "b depends on unfinished a")
}

func TestIntegration_GetUnfinishedTasks_EffectiveDueAtPropagatesBackward(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	earlyDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	lateDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	a, err := repo.CreateTask(ctx, nil, "a", nil, &lateDue, "standard", nil, 4)
	require.NoError(t, err)
	b, err := repo.CreateTask(ctx, nil, "b", nil, &earlyDue, "standard", nil, 4)
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, b.ID, []int32{a.ID}))

	rows, err := repo.GetUnfinishedTasks(ctx, nil)
	require.NoError(t, err)

	byID := make(map[int32]tasks.UnfinishedTask, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}

	require.NotNil(t, byID[a.ID].DueAt)
	require.Equal(t, earlyDue, byID[a.ID].DueAt.UTC(), "a inherits b's earlier due_at since a blocks b")
	require.NotNil(t, byID[b.ID].DueAt)
	require.Equal(t, earlyDue, byID[b.ID].DueAt.UTC(), "b keeps its own earlier due_at")
}

func TestIntegration_GetTasksByDueDate_HiddenFilteredAndOrderedByEffective(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	earlyDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	lateDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	a, err := repo.CreateTask(ctx, nil, "a", nil, &lateDue, "standard", nil, 4)
	require.NoError(t, err)
	b, err := repo.CreateTask(ctx, nil, "b", nil, &earlyDue, "standard", nil, 4)
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, b.ID, []int32{a.ID}))

	c, err := repo.CreateTask(ctx, nil, "c", nil, &lateDue, "standard", nil, 4)
	require.NoError(t, err)
	d, err := repo.CreateTask(ctx, nil, "d", nil, nil, "standard", nil, 4)
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceTaskDependencies(ctx, d.ID, []int32{c.ID}))

	rows, err := repo.GetTasksByDueDate(ctx, nil)
	require.NoError(t, err)

	ids := make([]int32, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	require.NotContains(t, ids, d.ID, "d is hidden (only dep c is blocked) and has no own due_at")
	require.Contains(t, ids, a.ID, "a appears with inherited earlier due")
	require.Contains(t, ids, b.ID)
	require.Contains(t, ids, c.ID, "c appears (own due_at present) even though blocking d")

	posA := indexOf(ids, a.ID)
	posB := indexOf(ids, b.ID)
	posC := indexOf(ids, c.ID)
	require.Less(t, posA, posC, "a sorts before c (effective due 2026-06-15 < c's 2026-12-31)")
	require.Less(t, posB, posC, "b sorts before c")
	_ = posA
	_ = posB
}

func indexOf(ids []int32, target int32) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

func TestIntegration_GetTasksByDueDate_MinPriorityFilter(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(t)

	due := time.Now().Add(24 * time.Hour)
	mkP := func(name string, priority int32) int32 {
		task, err := repo.CreateTask(ctx, nil, name, nil, &due, "standard", nil, priority)
		require.NoError(t, err)
		return task.ID
	}
	mkP("urgent", 1)
	mkP("normal", 3)
	mkP("low", 5)

	all, err := repo.GetTasksByDueDate(ctx, nil)
	require.NoError(t, err)
	require.Len(t, all, 3)

	threshold := int32(2)
	filtered, err := repo.GetTasksByDueDate(ctx, &threshold)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "urgent", filtered[0].Name)
}
