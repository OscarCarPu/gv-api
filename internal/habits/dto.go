package habits

// Responses and requests

type HabitWithLog struct {
	ID                int32    `json:"id"`
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	Frequency         string   `json:"frequency"`
	TargetMin         *float32 `json:"target_min"`
	TargetMax         *float32 `json:"target_max"`
	RecordingRequired bool     `json:"recording_required"`
	LogValue          *float32 `json:"log_value"`
	PeriodValue       float32  `json:"period_value"`
	CurrentStreak     int32    `json:"current_streak"`
	LongestStreak     int32    `json:"longest_streak"`
}

type LogUpsertRequest struct {
	HabitID int32   `json:"habit_id"`
	Date    string  `json:"date"`
	Value   float32 `json:"value"`
}

type CreateHabitRequest struct {
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	Frequency         *string  `json:"frequency"`
	TargetMin         *float32 `json:"target_min"`
	TargetMax         *float32 `json:"target_max"`
	RecordingRequired *bool    `json:"recording_required"`
}

type HistoryPoint struct {
	Date  string  `json:"date"`
	Value float32 `json:"value"`
}

type HistoryResponse struct {
	StartAt string         `json:"start_at"`
	EndAt   string         `json:"end_at"`
	Data    []HistoryPoint `json:"data"`
}

type CreateHabitResponse struct {
	ID                int32    `json:"id"`
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	Frequency         string   `json:"frequency"`
	TargetMin         *float32 `json:"target_min"`
	TargetMax         *float32 `json:"target_max"`
	RecordingRequired bool     `json:"recording_required"`
	CurrentStreak     int32    `json:"current_streak"`
	LongestStreak     int32    `json:"longest_streak"`
}
