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

func ceilToMonday(d time.Time) time.Time {
	if d.Weekday() == time.Monday {
		return d
	}
	offset := (int(time.Monday) - int(d.Weekday()) + 7) % 7
	return d.AddDate(0, 0, offset)
}

func ceilToFirstOfMonth(d time.Time) time.Time {
	if d.Day() == 1 {
		return d
	}
	return time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, d.Location())
}

// --- LogHabit ---

func TestService_LogHabit_HasTargets_CallsRecalculateStreak(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: ptr(float32(1)), RecordingRequired: true,
	}, nil)
	repo.EXPECT().RecalculateStreak(mock.Anything, int32(1), mock.Anything).Return(nil)

	err := svc.LogHabit(context.Background(), habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_NoTargets_SkipsStreak(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, int32(1), mock.Anything, float32(1)).Return(nil)
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", TargetMin: nil, TargetMax: nil, RecordingRequired: true,
	}, nil)
	// RecalculateStreak should NOT be called

	err := svc.LogHabit(context.Background(), habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	require.NoError(t, err)
}

func TestService_LogHabit_UpsertError_Propagates(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	repo.EXPECT().UpsertLog(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db error"))

	err := svc.LogHabit(context.Background(), habits.LogUpsertRequest{HabitID: 1, Date: "2026-03-17", Value: 1})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestService_LogHabit_InvalidDate_ReturnsError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)

	err := svc.LogHabit(context.Background(), habits.LogUpsertRequest{HabitID: 1, Date: "not-a-date", Value: 1})
	assert.Error(t, err)
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
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week", mock.Anything, mock.Anything, mock.Anything).
		Return([]habits.HistoryPoint{{Date: "2026-03-10", Value: 5}}, nil)

	result, err := svc.GetHistory(ctx, 1, "", "2026-03-01", "2026-03-19")
	require.NoError(t, err)
	// 2026-03-01 is a Sunday -> previous Monday is 2026-02-23; 2026-03-19 is a Thursday -> next Monday is 2026-03-23
	assert.Equal(t, "2026-02-23", result.StartAt)
	assert.Equal(t, "2026-03-23", result.EndAt)
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
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything, mock.Anything).
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
		mock.Anything,
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

	// start snaps to previous Monday, end snaps to next Monday (or stays if already Monday)
	mondayOfStart := truncateToMonday(twelveWeeksAgo)
	mondayOfEnd := ceilToMonday(today)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "weekly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week",
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(mondayOfStart) }),
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(mondayOfEnd) }),
		mock.Anything,
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

	// start snaps to 1st of month, end snaps to next 1st (or stays if already 1st)
	firstOfStartMonth := time.Date(twelveMonthsAgo.Year(), twelveMonthsAgo.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstOfEndMonth := ceilToFirstOfMonth(today)

	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "monthly", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "month",
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(firstOfStartMonth) }),
		mock.MatchedBy(func(t time.Time) bool { return t.Equal(firstOfEndMonth) }),
		mock.Anything,
	).Return([]habits.HistoryPoint{}, nil)

	result, err := svc.GetHistory(ctx, 1, "monthly", "", "")
	require.NoError(t, err)
	assert.Equal(t, firstOfStartMonth.Format("2006-01-02"), result.StartAt)
	assert.Equal(t, firstOfEndMonth.Format("2006-01-02"), result.EndAt)
}

func TestService_GetHistory_ForwardsRecordingRequiredAsFillZeros(t *testing.T) {
	t.Run("recording_required=true forwards fill_zeros=true", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := habits.NewService(repo, time.UTC)

		repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
			ID: 1, Frequency: "daily", RecordingRequired: true,
		}, nil)
		repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything, true).
			Return([]habits.HistoryPoint{}, nil)

		_, err := svc.GetHistory(context.Background(), 1, "daily", "2026-03-01", "2026-03-04")
		require.NoError(t, err)
	})

	t.Run("recording_required=false forwards fill_zeros=false", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := habits.NewService(repo, time.UTC)

		repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
			ID: 1, Frequency: "daily", RecordingRequired: false,
		}, nil)
		repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything, false).
			Return([]habits.HistoryPoint{}, nil)

		_, err := svc.GetHistory(context.Background(), 1, "daily", "2026-03-01", "2026-03-04")
		require.NoError(t, err)
	})
}

func TestService_GetHistory_CoarserFrequency_UsesAvg(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := habits.NewService(repo, time.UTC)
	ctx := context.Background()

	// Daily habit viewed as weekly → coarser → AVG
	repo.EXPECT().GetHabitByID(mock.Anything, int32(1)).Return(habitsdb.GetHabitByIDRow{
		ID: 1, Frequency: "daily", RecordingRequired: false,
	}, nil)
	repo.EXPECT().GetHabitHistoryAvg(mock.Anything, int32(1), "week", mock.Anything, mock.Anything, mock.Anything).
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
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "week", mock.Anything, mock.Anything, mock.Anything).
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
	repo.EXPECT().GetHabitHistory(mock.Anything, int32(1), "day", mock.Anything, mock.Anything, mock.Anything).
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
