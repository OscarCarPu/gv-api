package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// DTOs representing the API contract from the client's perspective.
type CreateHabitRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type CreateHabitResponse struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type HabitWithLog struct {
	ID          int32    `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	LogValue    *float32 `json:"log_value"`
}

type LogRequest struct {
	HabitID int32   `json:"habit_id"`
	Date    string  `json:"date"`
	Value   float32 `json:"value"`
}

// APIClient is a test driver that wraps HTTP calls to the API.
type APIClient struct {
	baseURL string
	http    *http.Client
}

func NewAPIClient(t *testing.T) *APIClient {
	t.Helper()
	return &APIClient{
		baseURL: getBaseURL(t),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *APIClient) CreateHabit(t *testing.T, req CreateHabitRequest) CreateHabitResponse {
	t.Helper()
	body, _ := json.Marshal(req)

	resp, err := c.http.Post(c.baseURL+"/habits", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("failed to create habit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d, want 201", resp.StatusCode)
	}

	var habit CreateHabitResponse
	if err := json.NewDecoder(resp.Body).Decode(&habit); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return habit
}

func (c *APIClient) LogHabit(t *testing.T, req LogRequest) {
	t.Helper()
	body, _ := json.Marshal(req)

	resp, err := c.http.Post(c.baseURL+"/habits/log", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("failed to log habit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
}

func (c *APIClient) GetDailyView(t *testing.T, date string) []HabitWithLog {
	t.Helper()

	resp, err := c.http.Get(fmt.Sprintf("%s/habits?date=%s", c.baseURL, date))
	if err != nil {
		t.Fatalf("failed to get daily view: %v", err)
	}
	defer resp.Body.Close()

	var habits []HabitWithLog
	if err := json.NewDecoder(resp.Body).Decode(&habits); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return habits
}
