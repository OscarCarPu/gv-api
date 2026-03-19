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

func truncateToMonday(d time.Time) time.Time {
	offset := (int(d.Weekday()) - int(time.Monday) + 7) % 7
	return d.AddDate(0, 0, -offset)
}

// --- LogHabit + streak recalculation ---

func TestService_LogHabit_RecalculatesStreak_Daily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	now := time.Now().In(time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)
	fourDaysAgo := today.AddDate(0, 0, -4)

	// 3 consecutive days meeting target_min=1, then a gap, then an older day
	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: true, LongestStreak: 5,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: today, Value: 1},
		{HabitID: 1, LogDate: yesterday, Value: 1},
		{HabitID: 1, LogDate: twoDaysAgo, Value: 1},
		// gap on 3 days ago
		{HabitID: 1, LogDate: fourDaysAgo, Value: 1},
	}, nil)
	// Current streak=3 (today + 2 previous), longest recalculated from all periods=3
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(3), int32(3)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: today.Format("2006-01-02"), Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_CurrentPeriodNotMet_SkipsWithoutBreaking(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := todayDate.AddDate(0, 0, -1)
	twoDaysAgo := todayDate.AddDate(0, 0, -2)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(0.5)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: true, LongestStreak: 0,
	}, nil)
	// Today has 0.5 (below target of 1), but yesterday and day before met it
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

func TestService_LogHabit_NoTargets_SkipsStreak(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: nil, TargetMax: nil, RecordingRequired: true, LongestStreak: 0,
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
	repo.EXPECT().GetHabitByID(mock.Anything, int32(2)).Return(habitsdb.GetHabitByIDRow{
		ID: 2, Frequency: "weekly", TargetMin: ptr(float32(3)), RecordingRequired: true, LongestStreak: 0,
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
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: true, LongestStreak: 2,
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
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: true, LongestStreak: 5,
	}, nil)
	// Today not met, yesterday not met either -> streak=0
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 0.5},
		// gap yesterday
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 1},
	}, nil)
	// Streak=0, longest recalculated from all periods=1 (only one day meets target)
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(0), int32(1)).Return(nil)

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

// --- Range target (target_max only) ---

func TestService_LogHabit_MaxOnlyTarget(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(5)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMax: ptr(float32(8)), RecordingRequired: true,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 5},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -1), Value: 7},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 6},
	}, nil)
	// All 3 days <= 8, streak=3
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(3), int32(3)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 5})
	require.NoError(t, err)
}

// --- Range target (both min and max) ---

func TestService_LogHabit_RangeTarget(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(70)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(60)), TargetMax: ptr(float32(80)), RecordingRequired: true,
	}, nil)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 70},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -1), Value: 65},
		// day -2: value 90 exceeds max
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 90},
	}, nil)
	// Today and yesterday in range -> streak=2, longest=2
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(2), int32(2)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 70})
	require.NoError(t, err)
}

// --- recording_required=false carry-forward ---

func TestService_LogHabit_RecordingNotRequired_CarryForward(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: false,
	}, nil)
	// Logs: day-3=1, day-2=missing, day-1=missing, today=1
	// With carry-forward: day-3=1, day-2=1(carried), day-1=1(carried), today=1
	// All meet target -> streak=4
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate, Value: 1},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -3), Value: 1},
	}, nil)
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(4), int32(4)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_RecordingNotRequired_CarryForwardBreaks(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.FixedZone("UTC", 0)
	svc := habits.NewService(repo, loc)
	ctx := context.Background()

	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(0)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: false,
	}, nil)
	// Logs: day-3=1, day-2=1, day-1=0, today=missing(carried 0)
	// day-1 and today don't meet target -> streak=0 (today skipped as current, day-1 breaks)
	// longest=2 (day-3, day-2)
	repo.EXPECT().GetHabitLogs(mock.Anything, int32(1)).Return([]habitsdb.HabitLog{
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -1), Value: 0},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -2), Value: 1},
		{HabitID: 1, LogDate: todayDate.AddDate(0, 0, -3), Value: 1},
	}, nil)
	repo.EXPECT().UpdateHabitStreak(mock.Anything, int32(1), int32(0), int32(2)).Return(nil)

	err := svc.LogHabit(ctx, habits.LogUpsertRequest{HabitID: 1, Date: todayDate.Format("2006-01-02"), Value: 0})
	require.NoError(t, err)
}

// --- CreateHabit ---

func TestService_CreateHabit_DefaultsFrequencyToDaily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().CreateHabit(mock.Anything, "Exercise", (*string)(nil), "daily", (*float32)(nil), (*float32)(nil), true).
		Return(habits.CreateHabitResponse{ID: 1, Name: "Exercise", Frequency: "daily", RecordingRequired: true}, nil)

	resp, err := svc.CreateHabit(ctx, habits.CreateHabitRequest{Name: "Exercise"})
	require.NoError(t, err)
	assert.Equal(t, "daily", resp.Frequency)
}

func TestService_CreateHabit_UsesProvidedFrequency(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	tmin := float32(5)
	repo.EXPECT().CreateHabit(mock.Anything, "Meditate", (*string)(nil), "weekly", &tmin, (*float32)(nil), true).
		Return(habits.CreateHabitResponse{ID: 1, Name: "Meditate", Frequency: "weekly", TargetMin: &tmin, RecordingRequired: true}, nil)

	resp, err := svc.CreateHabit(ctx, habits.CreateHabitRequest{
		Name: "Meditate", Frequency: ptr("weekly"), TargetMin: &tmin,
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

// --- GetHistory ---

func TestService_GetHistory_DefaultFrequencyFromHabit(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{{Date: "2026-03-10", Value: 5}}, nil)

	result, err := svc.GetHistory(ctx, 1, "", "2026-03-01", "2026-03-19")
	require.NoError(t, err)
	// 2026-03-01 is a Sunday -> Monday is 2026-02-23; 2026-03-19 is a Thursday -> Monday is 2026-03-16
	assert.Equal(t, "2026-02-23", result.StartAt)
	assert.Equal(t, "2026-03-16", result.EndAt)
	assert.Len(t, result.Data, 1)
	assert.Equal(t, "2026-03-10", result.Data[0].Date)
}

func TestService_GetHistory_ExplicitFrequency(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{}, nil)

	result, err := svc.GetHistory(ctx, 1, "daily", "2026-03-01", "2026-03-19")
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", result.StartAt)
	assert.Equal(t, "2026-03-19", result.EndAt)
	assert.Empty(t, result.Data)
}

func TestService_GetHistory_DefaultDates_Daily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	now := time.Now().In(time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	oneMonthAgo := today.AddDate(0, -1, 0)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day",
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(oneMonthAgo) }),
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(today) }),
	).Return([]habits.HistoryPoint{}, nil)

	result, err := svc.GetHistory(ctx, 1, "daily", "", "")
	require.NoError(t, err)
	assert.Equal(t, oneMonthAgo.Format("2006-01-02"), result.StartAt)
	assert.Equal(t, today.Format("2006-01-02"), result.EndAt)
}

func TestService_GetHistory_DefaultDates_Weekly(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	now := time.Now().In(time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	twelveWeeksAgo := today.AddDate(0, 0, -12*7)

	// periodStart truncates to Monday
	mondayOfStart := truncateToMonday(twelveWeeksAgo)
	mondayOfEnd := truncateToMonday(today)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week",
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(twelveWeeksAgo) }),
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(today) }),
	).Return([]habits.HistoryPoint{}, nil)

	result, err := svc.GetHistory(ctx, 1, "weekly", "", "")
	require.NoError(t, err)
	assert.Equal(t, mondayOfStart.Format("2006-01-02"), result.StartAt)
	assert.Equal(t, mondayOfEnd.Format("2006-01-02"), result.EndAt)
}

func TestService_GetHistory_DefaultDates_Monthly(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	now := time.Now().In(time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	twelveMonthsAgo := today.AddDate(-1, 0, 0)

	// periodStart truncates to 1st of month
	firstOfStartMonth := time.Date(twelveMonthsAgo.Year(), twelveMonthsAgo.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstOfEndMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "monthly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "month",
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(twelveMonthsAgo) }),
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(today) }),
	).Return([]habits.HistoryPoint{}, nil)

	result, err := svc.GetHistory(ctx, 1, "monthly", "", "")
	require.NoError(t, err)
	assert.Equal(t, firstOfStartMonth.Format("2006-01-02"), result.StartAt)
	assert.Equal(t, firstOfEndMonth.Format("2006-01-02"), result.EndAt)
}

func TestService_GetHistory_RecordingRequired_FillsZeros_Daily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: true,
	}, nil)
	// Only 2 of the 4 days have data
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-01", Value: 5},
			{Date: "2026-03-03", Value: 3},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "daily", "2026-03-01", "2026-03-04")
	require.NoError(t, err)
	require.Len(t, result.Data, 4)
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-01", Value: 5}, result.Data[0])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-02", Value: 0}, result.Data[1])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-03", Value: 3}, result.Data[2])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-04", Value: 0}, result.Data[3])
}

func TestService_GetHistory_RecordingRequired_FillsZeros_Weekly(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: true,
	}, nil)
	// 2026-03-02 is a Monday, 2026-03-16 is a Monday — 3 weeks
	// Only week 1 and 3 have data
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-02", Value: 10},
			{Date: "2026-03-16", Value: 7},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "weekly", "2026-03-02", "2026-03-16")
	require.NoError(t, err)
	require.Len(t, result.Data, 3)
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-02", Value: 10}, result.Data[0])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-09", Value: 0}, result.Data[1])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-16", Value: 7}, result.Data[2])
}

func TestService_GetHistory_RecordingRequired_FillsZeros_Monthly(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "monthly", RecordingRequired: true,
	}, nil)
	// Jan to Mar — only Jan and Mar have data
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "month", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-01-01", Value: 55},
			{Date: "2026-03-01", Value: 42},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "monthly", "2026-01-15", "2026-03-19")
	require.NoError(t, err)
	require.Len(t, result.Data, 3)
	assert.Equal(t, habits.HistoryPoint{Date: "2026-01-01", Value: 55}, result.Data[0])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-02-01", Value: 0}, result.Data[1])
	assert.Equal(t, habits.HistoryPoint{Date: "2026-03-01", Value: 42}, result.Data[2])
}

func TestService_GetHistory_RecordingNotRequired_NoZeroFill(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-01", Value: 70.5},
			{Date: "2026-03-03", Value: 71.0},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "daily", "2026-03-01", "2026-03-04")
	require.NoError(t, err)
	// No zero-fill: only the 2 points that have data
	assert.Len(t, result.Data, 2)
}

func TestService_GetHistory_CoarserFrequency_UsesAvg(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	// Daily habit viewed as weekly → coarser → AVG
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistoryAvg(mock.Anything, int32(1), "week", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-02", Value: 7.5},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "weekly", "2026-03-02", "2026-03-08")
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	assert.Equal(t, float32(7.5), result.Data[0].Value)
}

func TestService_GetHistory_SameFrequency_UsesSum(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	// Weekly habit viewed as weekly → same → SUM
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-02", Value: 15},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "weekly", "2026-03-02", "2026-03-08")
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	assert.Equal(t, float32(15), result.Data[0].Value)
}

func TestService_GetHistory_FinerFrequency_UsesSum(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	// Weekly habit viewed as daily → finer → SUM
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{
			{Date: "2026-03-02", Value: 3},
			{Date: "2026-03-04", Value: 5},
		}, nil)

	result, err := svc.GetHistory(ctx, 1, "daily", "2026-03-02", "2026-03-08")
	require.NoError(t, err)
	assert.Len(t, result.Data, 2)
}

func TestService_GetHistory_InvalidFrequency(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: true,
	}, nil)

	result, err := svc.GetHistory(context.Background(), 1, "yearly", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frequency")
	assert.Equal(t, habits.HistoryResponse{}, result)
}

func TestService_GetHistory_InvalidStartAt(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: true,
	}, nil)

	_, err := svc.GetHistory(context.Background(), 1, "daily", "bad", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_at date")
}

func TestService_GetHistory_InvalidEndAt(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: true,
	}, nil)

	_, err := svc.GetHistory(context.Background(), 1, "daily", "", "bad")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid end_at date")
}
