package e2e

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

// E2E tests prove the wiring (DB, router, middleware, handler) works end-to-end
// for representative user flows. Business rules and error branches are covered
// at lower test levels; this file deliberately stays narrow.

func TestE2E_ProjectAndTaskLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	proj := client.CreateProject(t, CreateProjectRequest{Name: "Project"})
	client.UpdateProject(t, proj.ID, UpdateProjectRequest{StartedAt: &now})

	task := client.CreateTask(t, CreateTaskRequest{Name: "Task", ProjectID: &proj.ID})
	client.UpdateTask(t, task.ID, UpdateTaskRequest{StartedAt: &now})

	finishedAt := time.Now().UTC().Truncate(time.Second)
	updated := client.UpdateTask(t, task.ID, UpdateTaskRequest{FinishedAt: &finishedAt})
	if updated.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}

	got := client.GetTask(t, task.ID)
	if got.ID != task.ID {
		t.Errorf("got task id %d, want %d", got.ID, task.ID)
	}
}

func TestE2E_TaskDependencyChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateTables(t)
	client := authenticate(t)

	a := client.CreateTask(t, CreateTaskRequest{Name: "A"})
	b := client.CreateTask(t, CreateTaskRequest{Name: "B", DependsOn: []int32{a.ID}})

	got := client.GetTask(t, b.ID)
	if len(got.DependsOn) != 1 || got.DependsOn[0].ID != a.ID {
		t.Fatalf("expected B to depend on A, got deps=%+v", got.DependsOn)
	}
	if !got.Blocked {
		t.Error("expected B to be blocked while A is unfinished")
	}

	// Finish A, B should unblock.
	now := time.Now().UTC().Truncate(time.Second)
	client.UpdateTask(t, a.ID, UpdateTaskRequest{StartedAt: &now, FinishedAt: &now})

	got = client.GetTask(t, b.ID)
	if got.Blocked {
		t.Error("expected B to be unblocked after A finished")
	}
}

func TestE2E_ClearDueAtViaPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateTables(t)
	client := authenticate(t)

	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	task := client.CreateTask(t, CreateTaskRequest{Name: "T", DueAt: &due})
	if task.DueAt == nil {
		t.Fatal("setup: expected due_at to be set on creation")
	}

	// The typed UpdateTaskRequest uses omitempty and cannot express explicit null,
	// so this case (regression for commit 9eaa587) needs a raw PATCH body.
	resp := client.do(t, http.MethodPatch, "/tasks/tasks/"+strconv.Itoa(int(task.ID)), []byte(`{"due_at": null}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear due_at: got %d, want 200", resp.StatusCode)
	}

	got := client.GetTask(t, task.ID)
	if got.DueAt != nil {
		t.Errorf("expected due_at cleared, got %v", got.DueAt)
	}
}

func TestE2E_AuthRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	client := NewAPIClient(t) // no token

	resp := client.do(t, http.MethodGet, "/tasks/projects", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for unauthenticated request", resp.StatusCode)
	}
}

func TestE2E_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateTables(t)
	client := authenticate(t)

	resp := client.do(t, http.MethodGet, "/tasks/tasks/999999", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404 for missing task", resp.StatusCode)
	}
}

