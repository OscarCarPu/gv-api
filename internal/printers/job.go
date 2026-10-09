package printers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type stopOutcome int

const (
	stopped stopOutcome = iota
	stopIdle
	stopRejected
)

func stopMessage(status int) string {
	switch status {
	case http.StatusConflict:
		return "The printer will not stop the job in its current state"
	case http.StatusNotFound:
		return "That job had already finished"
	case http.StatusUnauthorized:
		return "PrusaLink rejected the credentials"
	}
	return fmt.Sprintf("PrusaLink returned %d", status)
}

func (l *link) currentJobID(ctx context.Context) (int, bool, error) {
	status, data, err := l.do(ctx, request{
		method:  http.MethodGet,
		path:    "/api/v1/job",
		headers: map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return 0, false, err
	}
	if status == http.StatusNoContent {
		return 0, false, nil
	}
	if status != http.StatusOK {
		return 0, false, fmt.Errorf("PrusaLink /api/v1/job -> %d", status)
	}
	var job struct {
		ID *int `json:"id"`
	}
	if err := json.Unmarshal(data, &job); err != nil || job.ID == nil {
		return 0, false, err
	}
	return *job.ID, true, nil
}

func (l *link) stopJob(ctx context.Context) (stopOutcome, int, error) {
	id, running, err := l.currentJobID(ctx)
	if err != nil {
		return 0, 0, err
	}
	if !running {
		return stopIdle, 0, nil
	}
	status, _, err := l.do(ctx, request{
		method:  http.MethodDelete,
		path:    fmt.Sprintf("/api/v1/job/%d", id),
		headers: map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return 0, 0, err
	}
	if status < 200 || status > 299 {
		return stopRejected, status, nil
	}
	return stopped, 0, nil
}
