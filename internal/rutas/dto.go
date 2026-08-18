package rutas

import "time"

type ConcelloMark struct {
	ID          int32     `json:"id"`
	Name        string    `json:"name"`
	VisitedOn   time.Time `json:"visited_on"`
	Description string    `json:"description"`
}

type CreateMarkRequest struct {
	Name        string `json:"name"`
	VisitedOn   string `json:"visited_on"` // YYYY-MM-DD
	Description string `json:"description"`
}

type UpdateMarkRequest struct {
	ID          int32
	VisitedOn   string `json:"visited_on"` // YYYY-MM-DD
	Description string `json:"description"`
}
