package tasks_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gv-api/internal/database/tasksdb"
	"gv-api/internal/tasks"
	testutil "gv-api/internal/testutils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Benchmarks exercise the SQL paths that absorbed Go-side logic (recursive
// CTE for blocks-closure, json_agg for todos, bottom-up time accumulation).
// Run with: go test -bench=. -run=NONE -benchtime=10x ./internal/tasks/...
// Requires TEST_DB_URL.

func benchRepo(b *testing.B) (*tasks.PostgresRepository, *pgxpool.Pool) {
	b.Helper()
	pool := testutil.NewPool(b)
	testutil.Truncate(b, pool, "task_dependencies", "time_entries", "todos", "tasks", "projects")
	return tasks.NewRepository(tasksdb.New(pool)), pool
}

// seedTasks creates `n` standalone unfinished tasks. Returns their IDs in
// creation order. With dependencies, downstream IDs depend on earlier ones.
func seedTasks(b *testing.B, repo *tasks.PostgresRepository, n int) []int32 {
	b.Helper()
	ctx := context.Background()
	due := time.Now().Add(72 * time.Hour)
	ids := make([]int32, n)
	for i := 0; i < n; i++ {
		t, err := repo.CreateTask(ctx, nil, fmt.Sprintf("t%d", i), nil, &due, "standard", nil, 3)
		if err != nil {
			b.Fatal(err)
		}
		ids[i] = t.ID
	}
	return ids
}

// seedDependencyChain wires t[i] depends_on t[i-1] for the first `chainLen` tasks.
func seedDependencyChain(b *testing.B, repo *tasks.PostgresRepository, ids []int32, chainLen int) {
	b.Helper()
	ctx := context.Background()
	for i := 1; i < chainLen && i < len(ids); i++ {
		if err := repo.ReplaceTaskDependencies(ctx, ids[i], []int32{ids[i-1]}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetUnfinishedTasks(b *testing.B) {
	cases := []struct {
		name     string
		tasks    int
		chainLen int
	}{
		{"100tasks_noDeps", 100, 0},
		{"1000tasks_noDeps", 1000, 0},
		{"100tasks_50chain", 100, 50},
		{"1000tasks_100chain", 1000, 100},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			repo, _ := benchRepo(b)
			ids := seedTasks(b, repo, tc.tasks)
			seedDependencyChain(b, repo, ids, tc.chainLen)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := repo.GetUnfinishedTasks(ctx, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGetTasksByDueDate(b *testing.B) {
	cases := []struct {
		name     string
		tasks    int
		chainLen int
	}{
		{"100tasks_noDeps", 100, 0},
		{"1000tasks_noDeps", 1000, 0},
		{"1000tasks_100chain", 1000, 100},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			repo, _ := benchRepo(b)
			ids := seedTasks(b, repo, tc.tasks)
			seedDependencyChain(b, repo, ids, tc.chainLen)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := repo.GetTasksByDueDate(ctx, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGetProjectChildren exercises the recursive descendants CTE plus
// the bottom-up time accumulation. Builds a chain of `depth` nested projects,
// each with `tasksPerProject` tasks, each task with one finished time entry.
func BenchmarkGetProjectChildren(b *testing.B) {
	cases := []struct {
		name            string
		depth           int
		tasksPerProject int
	}{
		{"depth5_5tasks", 5, 5},
		{"depth10_10tasks", 10, 10},
		{"depth20_20tasks", 20, 20},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			repo, _ := benchRepo(b)
			ctx := context.Background()
			now := time.Now().UTC()

			rootP, err := repo.CreateProject(ctx, "root", nil, nil, nil)
			if err != nil {
				b.Fatal(err)
			}
			parentID := rootP.ID
			for d := 0; d < tc.depth; d++ {
				p, err := repo.CreateProject(ctx, fmt.Sprintf("p%d", d), nil, nil, &parentID)
				if err != nil {
					b.Fatal(err)
				}
				for ti := 0; ti < tc.tasksPerProject; ti++ {
					task, err := repo.CreateTask(ctx, &p.ID, fmt.Sprintf("t%d-%d", d, ti), nil, nil, "standard", nil, 3)
					if err != nil {
						b.Fatal(err)
					}
					end := now.Add(time.Duration(60+ti) * time.Second)
					if _, err := repo.CreateTimeEntry(ctx, task.ID, now, &end, nil); err != nil {
						b.Fatal(err)
					}
				}
				parentID = p.ID
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := repo.GetProjectChildren(ctx, rootP.ID); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
