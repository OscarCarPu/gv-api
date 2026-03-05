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

// Tasks DTOs

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	ParentID    *int32  `json:"parent_id,omitempty"`
}

type ProjectResponse struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	ParentID    *int32  `json:"parent_id"`
	StartedAt   *string `json:"started_at"`
	FinishedAt  *string `json:"finished_at"`
}

type CreateTaskRequest struct {
	ProjectID   *int32  `json:"project_id,omitempty"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type TaskResponse struct {
	ID          int32   `json:"id"`
	ProjectID   *int32  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	StartedAt   *string `json:"started_at"`
	FinishedAt  *string `json:"finished_at"`
}

type CreateTodoRequest struct {
	TaskID int32  `json:"task_id"`
	Name   string `json:"name"`
}

type TodoResponse struct {
	ID     int32  `json:"id"`
	TaskID int32  `json:"task_id"`
	Name   string `json:"name"`
	IsDone bool   `json:"is_done"`
}

type CreateTimeEntryRequest struct {
	TaskID     int32   `json:"task_id"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at,omitempty"`
	Comment    *string `json:"comment,omitempty"`
}

type TimeEntryResponse struct {
	ID         int32   `json:"id"`
	TaskID     int32   `json:"task_id"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Comment    *string `json:"comment"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ParentID    *int32  `json:"parent_id,omitempty"`
	StartedAt   *string `json:"started_at,omitempty"`
	FinishedAt  *string `json:"finished_at,omitempty"`
}

type UpdateTaskRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ProjectID   *int32  `json:"project_id,omitempty"`
	StartedAt   *string `json:"started_at,omitempty"`
	FinishedAt  *string `json:"finished_at,omitempty"`
}

type UpdateTodoRequest struct {
	Name   *string `json:"name,omitempty"`
	IsDone *bool   `json:"is_done,omitempty"`
}

type UpdateTimeEntryRequest struct {
	StartedAt  *string `json:"started_at,omitempty"`
	FinishedAt *string `json:"finished_at,omitempty"`
	Comment    *string `json:"comment,omitempty"`
}

type TaskDetailResponse struct {
	ID          int32   `json:"id"`
	ProjectID   *int32  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	StartedAt   *string `json:"started_at"`
	FinishedAt  *string `json:"finished_at"`
	TimeSpent   int64   `json:"time_spent"`
}

type ProjectDetailResponse struct {
	ID          int32   `json:"id"`
	ParentID    *int32  `json:"parent_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	StartedAt   *string `json:"started_at"`
	FinishedAt  *string `json:"finished_at"`
	TimeSpent   int64   `json:"time_spent"`
}

type TaskTimeEntriesResponse struct {
	Task        TaskDetailResponse  `json:"task"`
	TimeEntries []TimeEntryResponse `json:"time_entries"`
}

type ProjectChildNode struct {
	ID          int32          `json:"id"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	StartedAt   *string        `json:"started_at"`
	FinishedAt  *string        `json:"finished_at"`
	TimeSpent   int64          `json:"time_spent"`
	ParentID    *int32         `json:"parent_id,omitempty"`
	ProjectID   *int32         `json:"project_id,omitempty"`
	Todos       []TodoResponse `json:"todos,omitempty"`
}

type ProjectChildrenResponse struct {
	Project  ProjectDetailResponse `json:"project"`
	Children []ProjectChildNode    `json:"children"`
}

type ActiveTreeNode struct {
	ID       int32            `json:"id"`
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	Children []ActiveTreeNode `json:"children,omitempty"`
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

// Tasks client methods

func (c *APIClient) CreateProject(t *testing.T, req CreateProjectRequest) ProjectResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPost, "/tasks/projects", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CreateProject: got status %d, want 201", resp.StatusCode)
	}
	var out ProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("CreateProject: decode: %v", err)
	}
	return out
}

func (c *APIClient) GetRootProjects(t *testing.T) []ProjectResponse {
	t.Helper()
	resp := c.do(t, http.MethodGet, "/tasks/projects", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetRootProjects: got status %d, want 200", resp.StatusCode)
	}
	var out []ProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("GetRootProjects: decode: %v", err)
	}
	return out
}

func (c *APIClient) GetProjectChildren(t *testing.T, id int32) ProjectChildrenResponse {
	t.Helper()
	resp := c.do(t, http.MethodGet, fmt.Sprintf("/tasks/projects/%d/children", id), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetProjectChildren: got status %d, want 200", resp.StatusCode)
	}
	var out ProjectChildrenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("GetProjectChildren: decode: %v", err)
	}
	return out
}

func (c *APIClient) UpdateProject(t *testing.T, id int32, req UpdateProjectRequest) ProjectResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPatch, fmt.Sprintf("/tasks/projects/%d", id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateProject: got status %d, want 200", resp.StatusCode)
	}
	var out ProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("UpdateProject: decode: %v", err)
	}
	return out
}

func (c *APIClient) CreateTask(t *testing.T, req CreateTaskRequest) TaskResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPost, "/tasks/tasks", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CreateTask: got status %d, want 201", resp.StatusCode)
	}
	var out TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("CreateTask: decode: %v", err)
	}
	return out
}

func (c *APIClient) GetTaskTimeEntries(t *testing.T, id int32) TaskTimeEntriesResponse {
	t.Helper()
	resp := c.do(t, http.MethodGet, fmt.Sprintf("/tasks/tasks/%d/time-entries", id), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetTaskTimeEntries: got status %d, want 200", resp.StatusCode)
	}
	var out TaskTimeEntriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("GetTaskTimeEntries: decode: %v", err)
	}
	return out
}

func (c *APIClient) UpdateTask(t *testing.T, id int32, req UpdateTaskRequest) TaskResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPatch, fmt.Sprintf("/tasks/tasks/%d", id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateTask: got status %d, want 200", resp.StatusCode)
	}
	var out TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("UpdateTask: decode: %v", err)
	}
	return out
}

func (c *APIClient) CreateTodo(t *testing.T, req CreateTodoRequest) TodoResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPost, "/tasks/todos", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CreateTodo: got status %d, want 201", resp.StatusCode)
	}
	var out TodoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("CreateTodo: decode: %v", err)
	}
	return out
}

func (c *APIClient) UpdateTodo(t *testing.T, id int32, req UpdateTodoRequest) TodoResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPatch, fmt.Sprintf("/tasks/todos/%d", id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateTodo: got status %d, want 200", resp.StatusCode)
	}
	var out TodoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("UpdateTodo: decode: %v", err)
	}
	return out
}

func (c *APIClient) CreateTimeEntry(t *testing.T, req CreateTimeEntryRequest) TimeEntryResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPost, "/tasks/time-entries", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CreateTimeEntry: got status %d, want 201", resp.StatusCode)
	}
	var out TimeEntryResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("CreateTimeEntry: decode: %v", err)
	}
	return out
}

func (c *APIClient) UpdateTimeEntry(t *testing.T, id int32, req UpdateTimeEntryRequest) TimeEntryResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp := c.do(t, http.MethodPatch, fmt.Sprintf("/tasks/time-entries/%d", id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateTimeEntry: got status %d, want 200", resp.StatusCode)
	}
	var out TimeEntryResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("UpdateTimeEntry: decode: %v", err)
	}
	return out
}

func (c *APIClient) GetActiveTree(t *testing.T) []ActiveTreeNode {
	t.Helper()
	resp := c.do(t, http.MethodGet, "/tasks/tree", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetActiveTree: got status %d, want 200", resp.StatusCode)
	}
	var out []ActiveTreeNode
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("GetActiveTree: decode: %v", err)
	}
	return out
}
