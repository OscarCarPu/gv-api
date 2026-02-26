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
	token   string
	http    *http.Client
}

func NewAPIClient(t *testing.T) *APIClient {
	t.Helper()
	return &APIClient{
		baseURL: getBaseURL(t),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *APIClient) SetToken(token string) {
	c.token = token
}

func (c *APIClient) do(t *testing.T, method, path string, body []byte) *http.Response {
	t.Helper()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, c.baseURL+path, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, c.baseURL+path, nil)
	}
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func (c *APIClient) Login(t *testing.T, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	resp := c.do(t, http.MethodPost, "/login", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: got status %d, want 200", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("login: failed to decode response: %v", err)
	}
	return result["token"]
}

func (c *APIClient) Login2FA(t *testing.T, tmpToken, code string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": tmpToken, "code": code})
	resp := c.do(t, http.MethodPost, "/login/2fa", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("2fa: got status %d, want 200", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("2fa: failed to decode response: %v", err)
	}
	return result["token"]
}

func (c *APIClient) CreateHabit(t *testing.T, req CreateHabitRequest) CreateHabitResponse {
	t.Helper()
	body, _ := json.Marshal(req)

	resp := c.do(t, http.MethodPost, "/habits", body)
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

	resp := c.do(t, http.MethodPost, "/habits/log", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
}

func (c *APIClient) GetDailyView(t *testing.T, date string) []HabitWithLog {
	t.Helper()

	resp := c.do(t, http.MethodGet, fmt.Sprintf("/habits?date=%s", date), nil)
	defer resp.Body.Close()

	var habits []HabitWithLog
	if err := json.NewDecoder(resp.Body).Decode(&habits); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return habits
}
