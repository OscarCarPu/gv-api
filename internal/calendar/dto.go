package calendar

import "time"

type Event struct {
	ID        int32     `json:"id"`
	Name      string    `json:"name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type CreateEventRequest struct {
	Name      string    `json:"name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// UpdateEventRequest is a patch: absent fields are left alone.
type UpdateEventRequest struct {
	Name      *string    `json:"name"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}
