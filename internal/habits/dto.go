package habits

import "time"

// REsponses and requests

type HabitWithLog struct {
	ID          int32    `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	LogValue    *float32 `json:"log_value"`
}

type LogUpsertRequest struct {
	HabitID int32     `json:"habit_id"`
	Date    time.Time `json:"date"`
	Value   float32   `json:"value"`
}
