package habits

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gv-api/internal/database/habitsdb"
)

type Service struct {
	repo     Repository
	location *time.Location
}

func NewService(repo Repository, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{repo: repo, location: loc}
}

func (s *Service) GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
	targetDate := time.Now().In(s.location)

	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, err
		}
		targetDate = parsed
	}

	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)

	return s.repo.GetHabitsWithLogs(ctx, targetDate)
}

func (s *Service) LogHabit(ctx context.Context, req LogUpsertRequest) error {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return err
	}
	if err := s.repo.UpsertLog(ctx, req.HabitID, date, req.Value); err != nil {
		return err
	}
	return s.recalculateStreak(ctx, req.HabitID)
}

func (s *Service) CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
	frequency := "daily"
	if req.Frequency != nil && *req.Frequency != "" {
		frequency = *req.Frequency
	}
	if frequency != "daily" && frequency != "weekly" && frequency != "monthly" {
		return CreateHabitResponse{}, fmt.Errorf("invalid frequency: %s", frequency)
	}

	recordingRequired := true
	if req.RecordingRequired != nil {
		recordingRequired = *req.RecordingRequired
	}

	return s.repo.CreateHabit(ctx, req.Name, req.Description, frequency, req.TargetMin, req.TargetMax, recordingRequired)
}

func (s *Service) DeleteHabit(ctx context.Context, id int32) error {
	return s.repo.DeleteHabit(ctx, id)
}

// frequencyToTrunc maps API frequency names to PostgreSQL date_trunc names.
var frequencyToTrunc = map[string]string{
	"daily":   "day",
	"weekly":  "week",
	"monthly": "month",
}

// frequencyRank orders frequencies from finest to coarsest.
var frequencyRank = map[string]int{
	"daily":   0,
	"weekly":  1,
	"monthly": 2,
}

func defaultStartDate(today time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return today.AddDate(0, 0, -12*7)
	case "monthly":
		return today.AddDate(-1, 0, 0)
	default: // daily
		return today.AddDate(0, -1, 0)
	}
}

func (s *Service) GetHistory(ctx context.Context, habitID int32, frequency, startAt, endAt string) (HistoryResponse, error) {
	habit, err := s.repo.GetHabitByID(ctx, habitID)
	if err != nil {
		return HistoryResponse{}, err
	}

	if frequency == "" {
		frequency = habit.Frequency
	}

	trunc, ok := frequencyToTrunc[frequency]
	if !ok {
		return HistoryResponse{}, fmt.Errorf("invalid frequency: %s", frequency)
	}

	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	end := today
	if endAt != "" {
		parsed, err := time.Parse("2006-01-02", endAt)
		if err != nil {
			return HistoryResponse{}, fmt.Errorf("invalid end_at date")
		}
		end = parsed
	}

	start := defaultStartDate(today, frequency)
	if startAt != "" {
		parsed, err := time.Parse("2006-01-02", startAt)
		if err != nil {
			return HistoryResponse{}, fmt.Errorf("invalid start_at date")
		}
		start = parsed
	}

	// Snap start to the previous period boundary and end to the next one.
	start = periodStart(start, frequency)
	end = periodCeil(end, frequency)

	// Use AVG when viewing at a coarser frequency than the habit's native one.
	var data []HistoryPoint
	if frequencyRank[frequency] > frequencyRank[habit.Frequency] {
		data, err = s.repo.GetHabitHistoryAvg(ctx, habitID, trunc, start, end)
	} else {
		data, err = s.repo.GetHabitHistory(ctx, habitID, trunc, start, end)
	}
	if err != nil {
		return HistoryResponse{}, err
	}

	if habit.RecordingRequired {
		data = fillMissingPeriods(data, start, end, frequency)
	}

	return HistoryResponse{
		StartAt: start.Format("2006-01-02"),
		EndAt:   end.Format("2006-01-02"),
		Data:    data,
	}, nil
}

// fillMissingPeriods inserts zero-valued points for any periods in the range that have no data.
func fillMissingPeriods(data []HistoryPoint, start, end time.Time, frequency string) []HistoryPoint {
	existing := make(map[string]HistoryPoint, len(data))
	for _, p := range data {
		existing[p.Date] = p
	}

	var filled []HistoryPoint
	ps := periodStart(start, frequency)
	endPS := periodStart(end, frequency)
	for !ps.After(endPS) {
		key := ps.Format("2006-01-02")
		if p, ok := existing[key]; ok {
			filled = append(filled, p)
		} else {
			filled = append(filled, HistoryPoint{Date: key, Value: 0})
		}
		ps = nextPeriodStart(ps, frequency)
	}
	return filled
}

func (s *Service) recalculateStreak(ctx context.Context, habitID int32) error {
	habit, err := s.repo.GetHabitByID(ctx, habitID)
	if err != nil {
		return err
	}

	// No targets means no streak tracking
	if habit.TargetMin == nil && habit.TargetMax == nil {
		return nil
	}

	logs, err := s.repo.GetHabitLogs(ctx, habitID)
	if err != nil {
		return err
	}

	periodSums := buildEffectivePeriodSums(logs, habit.Frequency, habit.RecordingRequired, s.location)

	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	currentPeriod := periodStart(today, habit.Frequency)

	// Walk backwards counting consecutive periods that meet the target
	currentStreak := int32(0)
	period := currentPeriod
	for {
		sum, hasPeriod := periodSums[period]
		if hasPeriod && meetsTarget(sum, habit.TargetMin, habit.TargetMax) {
			currentStreak++
			period = previousPeriodStart(period, habit.Frequency)
			continue
		}
		// Current period not yet met — skip it, don't break
		if period.Equal(currentPeriod) {
			period = previousPeriodStart(period, habit.Frequency)
			continue
		}
		// Past period not met — streak is broken
		break
	}

	longestStreak := computeLongestStreak(periodSums, habit.TargetMin, habit.TargetMax, habit.Frequency)

	return s.repo.UpdateHabitStreak(ctx, habitID, currentStreak, longestStreak)
}

// buildEffectivePeriodSums returns period sums, applying carry-forward logic when recordingRequired is false.
func buildEffectivePeriodSums(logs []habitsdb.HabitLog, frequency string, recordingRequired bool, location *time.Location) map[time.Time]float32 {
	if recordingRequired {
		return groupLogsByPeriod(logs, frequency)
	}

	if len(logs) == 0 {
		return make(map[time.Time]float32)
	}

	// Sort logs by date ascending
	sorted := make([]habitsdb.HabitLog, len(logs))
	copy(sorted, logs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LogDate.Before(sorted[j].LogDate)
	})

	// Build a map of actual logged values by date
	loggedValues := make(map[time.Time]float32)
	for _, log := range sorted {
		d := time.Date(log.LogDate.Year(), log.LogDate.Month(), log.LogDate.Day(), 0, 0, 0, 0, time.UTC)
		loggedValues[d] += log.Value
	}

	earliest := time.Date(sorted[0].LogDate.Year(), sorted[0].LogDate.Month(), sorted[0].LogDate.Day(), 0, 0, 0, 0, time.UTC)
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Walk every day from earliest to today, carrying forward the last recorded value
	effectiveSums := make(map[time.Time]float32)
	var lastValue float32
	for d := earliest; !d.After(today); d = d.AddDate(0, 0, 1) {
		if v, ok := loggedValues[d]; ok {
			lastValue = v
		}
		ps := periodStart(d, frequency)
		effectiveSums[ps] += lastValue
	}

	return effectiveSums
}

// computeLongestStreak finds the longest run of consecutive periods meeting the target.
func computeLongestStreak(periodSums map[time.Time]float32, targetMin, targetMax *float32, frequency string) int32 {
	var metPeriods []time.Time
	for ps, sum := range periodSums {
		if meetsTarget(sum, targetMin, targetMax) {
			metPeriods = append(metPeriods, ps)
		}
	}
	if len(metPeriods) == 0 {
		return 0
	}

	sort.Slice(metPeriods, func(i, j int) bool {
		return metPeriods[i].Before(metPeriods[j])
	})

	longest := int32(1)
	current := int32(1)
	for i := 1; i < len(metPeriods); i++ {
		expected := nextPeriodStart(metPeriods[i-1], frequency)
		if metPeriods[i].Equal(expected) {
			current++
		} else {
			current = 1
		}
		if current > longest {
			longest = current
		}
	}
	return longest
}
