package habits_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/habits"
	"gv-api/internal/habits/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// --- LogHabit + streak recalculation ---

func TestService_LogHabit_RecalculatesStreak_Daily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	// 3 consecutive days meeting objective=1, then a gap
	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.Habit{
		ID: 1, Frequency: "daily", Objective: ptr(float32(1)), LongestStreak: 5,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: date(2026, 3, 17), Value: 1},
		{HabitID: 1, LogDate: date(2026, 3, 16), Value: 1},
		{HabitID: 1, LogDate: date(2026, 3, 15), Value: 1},
		// gap on 2026-03-14
		{HabitID: 1, LogDate: date(2026, 3, 13), Value: 1},
	}, nil)
	// Current streak should be 3 (today + 2 previous), longest stays 5
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(3), int32(5)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_CurrentPeriodNotMet_SkipsWithoutBreaking(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	// Use a fixed location so "today" is deterministic
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := todayDate.AddDate(0, 0, -1)
	twoDaysAgo := todayDate.AddDate(0, 0, -2)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(0.5)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.Habit{
		ID: 1, Frequency: "daily", Objective: ptr(float32(1)), LongestStreak: 0,
	}, nil)
	// Today has 0.5 (below objective of 1), but yesterday and day before met it
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 0.5},
		{HabitID: 1, LogDate: yesterday, Value: 1},
		{HabitID: 1, LogDate: twoDaysAgo, Value: 1.5},
	}, nil)
	// Current period (today) not met -> skip it, streak = 2 from yesterday+day before
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(2), int32(2)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 0.5})
	require.NoError(t, err)
}

func TestService_LogHabit_NoObjective_SkipsStreak(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.Habit{
		ID: 1, Frequency: "daily", Objective: nil, LongestStreak: 0,
	}, nil)
	// Should NOT call GetHabitLogs or UpdateHabitStreak

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_WeeklyStreak(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	// Find current week's Monday
	weekday := todayDate.Weekday()
	offset := (int(weekday) - int(time.Monday) + 7) % 7
	thisMonday := todayDate.AddDate(0, 0, -offset)
	lastMonday := thisMonday.AddDate(0, 0, -7)
	twoWeeksAgoMonday := thisMonday.AddDate(0, 0, -14)

	repo.EXPECT().UpsertLog(mock.Anything, int32(2), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(2)).Return(habitsdb.Habit{
		ID: 2, Frequency: "weekly", Objective: ptr(float32(3)), LongestStreak: 0,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(2)).Return([]habitsdb.HabitLog{
		// This week: 2 sessions (not yet meeting 3)
		{HabitID: 2, LogDate: thisMonday, Value: 1},
		{HabitID: 2, LogDate: thisMonday.AddDate(0, 0, 2), Value: 1},
		// Last week: 4 sessions (meets 3)
		{HabitID: 2, LogDate: lastMonday, Value: 1},
		{HabitID: 2, LogDate: lastMonday.AddDate(0, 0, 1), Value: 1},
		{HabitID: 2, LogDate: lastMonday.AddDate(0, 0, 3), Value: 1},
		{HabitID: 2, LogDate: lastMonday.AddDate(0, 0, 5), Value: 1},
		// Two weeks ago: 3 sessions (meets 3)
		{HabitID: 2, LogDate: twoWeeksAgoMonday, Value: 1},
		{HabitID: 2, LogDate: twoWeeksAgoMonday.AddDate(0, 0, 2), Value: 1},
		{HabitID: 2, LogDate: twoWeeksAgoMonday.AddDate(0, 0, 4), Value: 1},
	}, nil)
	// This week not met -> skip. Last 2 weeks met -> streak=2
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(2), int32(2), int32(2)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 2, Date: todayDate.Format("2006-01-02"), Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_LongestStreakUpdated(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.Habit{
		ID: 1, Frequency: "daily", Objective: ptr(float32(1)), LongestStreak: 2,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 1},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -1), Value: 1},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 1},
	}, nil)
	// Current streak=3 exceeds longest=2, so longest should update to 3
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(3), int32(3)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_BrokenStreak_ResetToZero(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(0.5)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.Habit{
		ID: 1, Frequency: "daily", Objective: ptr(float32(1)), LongestStreak: 5,
	}, nil)
	// Today not met, yesterday not met either -> streak=0
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 0.5},
		// gap yesterday
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 1},
	}, nil)
	// Streak=0, longest stays 5
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(0), int32(5)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 0.5})
	require.NoError(t, err)
}

func TestService_LogHabit_UpsertError_Propagates(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().UpsertLog(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db error"))

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestService_LogHabit_InvalidDate_ReturnsError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: "not-a-date", Value: 1})
	assert.Error(t, err)
}

// --- CreateHabit ---

func TestService_CreateHabit_DefaultsFrequencyToDaily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().CreateHabit(mock.Anything, "Exercise", (*string)(nil), "daily", (*float32)(nil)).
		Return(habits.CreateHabitResponse{ID: 1, Name: "Exercise", Frequency: "daily"}, nil)

	resp, err := svc.CreateHabit(ctx, habits.CreateHabitRequest{Name: "Exercise"})
	require.NoError(t, err)
	assert.Equal(t, "daily", resp.Frequency)
}

func TestService_CreateHabit_UsesProvidedFrequency(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	obj := float32(5)
	repo.EXPECT().CreateHabit(mock.Anything, "Meditate", (*string)(nil), "weekly", &obj).
		Return(habits.CreateHabitResponse{ID: 1, Name: "Meditate", Frequency: "weekly", Objective: &obj}, nil)

	resp, err := svc.CreateHabit(ctx, habits.CreateHabitRequest{
		Name: "Meditate", Frequency: ptr("weekly"), Objective: &obj,
	})
	require.NoError(t, err)
	assert.Equal(t, "weekly", resp.Frequency)
}

func TestService_CreateHabit_InvalidFrequency_ReturnsError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	_, err := svc.CreateHabit(ctx, habits.CreateHabitRequest{
		Name: "Bad", Frequency: ptr("yearly"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frequency")
}

// --- GetDailyView ---

func TestService_GetDailyView_DefaultsToToday(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitsWithLogs(mock.Anything, mock.Anything).Return([]habits.HabitWithLog{}, nil)

	result, err := svc.GetDailyView(ctx, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestService_GetDailyView_ParsesDateParam(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	expected := date(2026, 1, 15)
	repo.EXPECT().GetHabitsWithLogs(mock.Anything, expected).Return([]habits.HabitWithLog{}, nil)

	_, err := svc.GetDailyView(ctx, "2026-01-15")
	require.NoError(t, err)
}

func TestService_GetDailyView_InvalidDate_ReturnsError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	_, err := svc.GetDailyView(ctx, "bad-date")
	assert.Error(t, err)
}
