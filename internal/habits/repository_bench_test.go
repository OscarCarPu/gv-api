package habits_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/habits"
	testutil "gv-api/internal/testutils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Benchmarks for the SQL streak function and history zero-fill. The streak
// function is the heaviest read-side path for this domain (recursive CTE +
// gap-and-island over potentially years of daily logs).
// Run with: go test -bench=. -run=NONE -benchtime=10x ./internal/habits/...

func benchRepo(b *testing.B) (*habits.PostgresRepository, *pgxpool.Pool) {
	b.Helper()
	pool := testutil.NewPool(b)
	testutil.Truncate(b, pool, "habit_logs", "habits")
	return habits.NewRepository(habitsdb.New(pool)), pool
}

// seedDailyLogs creates `days` consecutive daily logs ending at today.
func seedDailyLogs(b *testing.B, repo *habits.PostgresRepository, habitID int32, days int) {
	b.Helper()
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for i := 0; i < days; i++ {
		if err := repo.UpsertLog(ctx, habitID, today.AddDate(0, 0, -i), 1); err != nil {
			b.Fatal(err)
		}
	}
}

// seedSparseLogs creates `count` logs spread across `spanDays` ending at today.
func seedSparseLogs(b *testing.B, repo *habits.PostgresRepository, habitID int32, count, spanDays int) {
	b.Helper()
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	step := spanDays / count
	if step < 1 {
		step = 1
	}
	for i := 0; i < count; i++ {
		if err := repo.UpsertLog(ctx, habitID, today.AddDate(0, 0, -i*step), 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecalculateStreak_Daily(b *testing.B) {
	cases := []int{30, 365, 1095}
	for _, days := range cases {
		b.Run(fmt.Sprintf("%dd", days), func(b *testing.B) {
			repo, _ := benchRepo(b)
			ctx := context.Background()
			tmin := float32(1)
			h, err := repo.CreateHabit(ctx, "h", nil, "daily", &tmin, nil, true)
			if err != nil {
				b.Fatal(err)
			}
			seedDailyLogs(b, repo, h.ID, days)
			today := time.Now().UTC().Truncate(24 * time.Hour)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := repo.RecalculateStreak(ctx, h.ID, today); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRecalculateStreak_Weekly(b *testing.B) {
	repo, _ := benchRepo(b)
	ctx := context.Background()
	tmin := float32(3)
	h, err := repo.CreateHabit(ctx, "h", nil, "weekly", &tmin, nil, true)
	if err != nil {
		b.Fatal(err)
	}
	// 2 years of daily logs -> ~104 weeks of period sums to scan
	seedDailyLogs(b, repo, h.ID, 730)
	today := time.Now().UTC().Truncate(24 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := repo.RecalculateStreak(ctx, h.ID, today); err != nil {
			b.Fatal(err)
		}
	}
}

// CarryForward is the worst case: per-day correlated subquery that scans
// backwards looking for the last logged value. With 1 year of sparse logs.
func BenchmarkRecalculateStreak_CarryForward(b *testing.B) {
	cases := []struct {
		name             string
		logs, spanDays   int
	}{
		{"30logs_365days", 30, 365},
		{"100logs_1095days", 100, 1095},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			repo, _ := benchRepo(b)
			ctx := context.Background()
			tmin := float32(1)
			h, err := repo.CreateHabit(ctx, "h", nil, "daily", &tmin, nil, false)
			if err != nil {
				b.Fatal(err)
			}
			seedSparseLogs(b, repo, h.ID, tc.logs, tc.spanDays)
			today := time.Now().UTC().Truncate(24 * time.Hour)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := repo.RecalculateStreak(ctx, h.ID, today); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGetHabitHistory_FillZeros(b *testing.B) {
	repo, _ := benchRepo(b)
	ctx := context.Background()
	h, err := repo.CreateHabit(ctx, "h", nil, "daily", nil, nil, true)
	if err != nil {
		b.Fatal(err)
	}
	seedDailyLogs(b, repo, h.ID, 365)
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(-1, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.GetHabitHistory(ctx, h.ID, "day", start, end, true); err != nil {
			b.Fatal(err)
		}
	}
}
