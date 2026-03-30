package e2e

import (
	"testing"
	"time"
)

func timePtr(t time.Time) *time.Time       { return &t }
func strPtr(s string) *string              { return &s }
func int32Ptr(i int32) *int32              { return &i }
func boolPtr(b bool) *bool                 { return &b }
func int32SlicePtr(s []int32) *[]int32     { return &s }

func TestE2E_PlanProjectHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	// Create root project
	root := client.CreateProject(t, CreateProjectRequest{Name: "Root Project"})

	// Create 2 sub-projects under root
	subA := client.CreateProject(t, CreateProjectRequest{Name: "Sub A", ParentID: &root.ID})
	subB := client.CreateProject(t, CreateProjectRequest{Name: "Sub B", ParentID: &root.ID})

	// Create tasks in each sub-project
	taskA := client.CreateTask(t, CreateTaskRequest{Name: "Task in A", ProjectID: &subA.ID})
	client.CreateTask(t, CreateTaskRequest{Name: "Task in B", ProjectID: &subB.ID})

	// GetRootProjects should return only root (sub-projects excluded)
	roots := client.GetRootProjects(t)
	found := false
	for _, p := range roots {
		if p.ID == subA.ID || p.ID == subB.ID {
			t.Errorf("sub-project %d should not appear in root projects", p.ID)
		}
		if p.ID == root.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("root project not found in GetRootProjects")
	}

	// GetProjectChildren(root) should show sub-projects as children
	children := client.GetProjectChildren(t, root.ID)
	if len(children.Children) < 2 {
		t.Fatalf("expected at least 2 children, got %d", len(children.Children))
	}
	subProjectCount := 0
	for _, c := range children.Children {
		if c.Type == "project" {
			subProjectCount++
		}
	}
	if subProjectCount != 2 {
		t.Errorf("expected 2 sub-project children, got %d", subProjectCount)
	}

	// GetProjectChildren(subA) should show taskA with empty todos
	subAChildren := client.GetProjectChildren(t, subA.ID)
	if len(subAChildren.Children) != 1 {
		t.Fatalf("expected 1 child in sub A, got %d", len(subAChildren.Children))
	}
	child := subAChildren.Children[0]
	if child.Type != "task" {
		t.Errorf("expected task type, got %s", child.Type)
	}
	if child.ID != taskA.ID {
		t.Errorf("expected task ID %d, got %d", taskA.ID, child.ID)
	}
	if len(child.Todos) != 0 {
		t.Errorf("expected 0 todos, got %d", len(child.Todos))
	}
}

func TestE2E_WorkOnTaskWithTimeTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Create and start project
	proj := client.CreateProject(t, CreateProjectRequest{Name: "Time Tracking Project"})
	client.UpdateProject(t, proj.ID, UpdateProjectRequest{StartedAt: &now})

	// Create and start task
	task := client.CreateTask(t, CreateTaskRequest{Name: "Tracked Task", ProjectID: &proj.ID})
	client.UpdateTask(t, task.ID, UpdateTaskRequest{StartedAt: &now})

	// Create open time entry, then close it (1.5h = 5400s)
	entryStart1 := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	entryEnd1 := time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)
	openEntry := client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:    task.ID,
		StartedAt: entryStart1,
	})
	client.UpdateTimeEntry(t, openEntry.ID, UpdateTimeEntryRequest{
		FinishedAt: &entryEnd1,
	})

	// Create already-finished time entry (1h = 3600s)
	entryStart2 := time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)
	entryEnd2 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     task.ID,
		StartedAt:  entryStart2,
		FinishedAt: &entryEnd2,
	})

	// Verify time entries and time_spent
	result := client.GetTaskTimeEntries(t, task.ID)
	if result.Task.TimeSpent != 9000 {
		t.Errorf("expected time_spent 9000, got %d", result.Task.TimeSpent)
	}
	if len(result.TimeEntries) != 2 {
		t.Errorf("expected 2 time entries, got %d", len(result.TimeEntries))
	}

	// Finish the task
	finishedAt := time.Now().UTC().Truncate(time.Second)
	updated := client.UpdateTask(t, task.ID, UpdateTaskRequest{FinishedAt: &finishedAt})
	if updated.FinishedAt == nil {
		t.Fatal("expected finished_at to be set after finishing task")
	}
}

func TestE2E_ManageTodosOnTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Create started project and task
	proj := client.CreateProject(t, CreateProjectRequest{Name: "Todo Project"})
	client.UpdateProject(t, proj.ID, UpdateProjectRequest{StartedAt: &now})
	task := client.CreateTask(t, CreateTaskRequest{Name: "Task with Todos", ProjectID: &proj.ID})

	// Create 3 todos
	todo1 := client.CreateTodo(t, CreateTodoRequest{TaskID: task.ID, Name: "Todo 1"})
	todo2 := client.CreateTodo(t, CreateTodoRequest{TaskID: task.ID, Name: "Todo 2"})
	todo3 := client.CreateTodo(t, CreateTodoRequest{TaskID: task.ID, Name: "Todo 3"})

	// All should start as not done
	if todo1.IsDone || todo2.IsDone || todo3.IsDone {
		t.Fatal("newly created todos should have is_done=false")
	}

	// Toggle 2 todos to done
	client.UpdateTodo(t, todo1.ID, UpdateTodoRequest{IsDone: boolPtr(true)})
	client.UpdateTodo(t, todo2.ID, UpdateTodoRequest{IsDone: boolPtr(true)})

	// Verify via GetProjectChildren
	children := client.GetProjectChildren(t, proj.ID)
	if len(children.Children) != 1 {
		t.Fatalf("expected 1 child task, got %d", len(children.Children))
	}
	taskNode := children.Children[0]
	if len(taskNode.Todos) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(taskNode.Todos))
	}
	doneCount := 0
	for _, td := range taskNode.Todos {
		if td.IsDone {
			doneCount++
		}
	}
	if doneCount != 2 {
		t.Errorf("expected 2 done todos, got %d", doneCount)
	}
}

func TestE2E_ActiveTreeFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Create active project (started, not finished)
	activeProj := client.CreateProject(t, CreateProjectRequest{Name: "Active Project"})
	client.UpdateProject(t, activeProj.ID, UpdateProjectRequest{StartedAt: &now})

	// Create unstarted project (should not appear)
	client.CreateProject(t, CreateProjectRequest{Name: "Unstarted Project"})

	// Create finished project (should not appear)
	finishedProj := client.CreateProject(t, CreateProjectRequest{Name: "Finished Project"})
	client.UpdateProject(t, finishedProj.ID, UpdateProjectRequest{
		StartedAt:  &now,
		FinishedAt: &now,
	})

	// Create sub-project under active, start it
	subProj := client.CreateProject(t, CreateProjectRequest{
		Name:     "Sub Project",
		ParentID: &activeProj.ID,
	})
	client.UpdateProject(t, subProj.ID, UpdateProjectRequest{StartedAt: &now})

	// Create tasks under active project: started, unstarted, finished
	startedTask := client.CreateTask(t, CreateTaskRequest{Name: "Started Task", ProjectID: &activeProj.ID})
	client.UpdateTask(t, startedTask.ID, UpdateTaskRequest{StartedAt: &now})

	client.CreateTask(t, CreateTaskRequest{Name: "Unstarted Task", ProjectID: &activeProj.ID})

	finishedTask := client.CreateTask(t, CreateTaskRequest{Name: "Finished Task", ProjectID: &activeProj.ID})
	client.UpdateTask(t, finishedTask.ID, UpdateTaskRequest{StartedAt: &now, FinishedAt: &now})

	// Create orphan task (no project), started
	orphanTask := client.CreateTask(t, CreateTaskRequest{Name: "Orphan Task"})
	client.UpdateTask(t, orphanTask.ID, UpdateTaskRequest{StartedAt: &now})

	// Get active tree
	tree := client.GetActiveTree(t)

	// Root should contain: active project + orphan task
	// (unstarted project, finished project excluded)
	var activeNode, orphanNode *ActiveTreeNode
	for i, n := range tree {
		switch n.ID {
		case activeProj.ID:
			activeNode = &tree[i]
		case orphanTask.ID:
			orphanNode = &tree[i]
		}
	}

	if activeNode == nil {
		t.Fatal("active project not found in tree root")
	}
	if orphanNode == nil {
		t.Fatal("orphan task not found in tree root")
	}
	if orphanNode.Type != "task" {
		t.Errorf("orphan node type: want task, got %s", orphanNode.Type)
	}

	// Active project children: sub-project first, then started task, then unstarted task; finished task absent
	if len(activeNode.Children) < 3 {
		t.Fatalf("expected at least 3 children in active project, got %d", len(activeNode.Children))
	}

	// First child should be sub-project (prepended)
	if activeNode.Children[0].Type != "project" || activeNode.Children[0].ID != subProj.ID {
		t.Errorf("first child should be sub-project %d, got type=%s id=%d",
			subProj.ID, activeNode.Children[0].Type, activeNode.Children[0].ID)
	}

	// Started task should come before unstarted
	startedIdx, unstartedIdx := -1, -1
	for i, c := range activeNode.Children {
		if c.ID == startedTask.ID {
			startedIdx = i
		}
		if c.Name == "Unstarted Task" {
			unstartedIdx = i
		}
		if c.ID == finishedTask.ID {
			t.Error("finished task should not appear in active tree")
		}
	}
	if startedIdx == -1 {
		t.Error("started task not found in active project children")
	}
	if unstartedIdx == -1 {
		t.Error("unstarted task not found in active project children")
	}
	if startedIdx > unstartedIdx {
		t.Errorf("started task (idx %d) should come before unstarted task (idx %d)", startedIdx, unstartedIdx)
	}
}

func TestE2E_TimeAccumulationAcrossHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Root -> Child A, Child B
	root := client.CreateProject(t, CreateProjectRequest{Name: "Root"})
	client.UpdateProject(t, root.ID, UpdateProjectRequest{StartedAt: &now})

	childA := client.CreateProject(t, CreateProjectRequest{Name: "Child A", ParentID: &root.ID})
	client.UpdateProject(t, childA.ID, UpdateProjectRequest{StartedAt: &now})

	childB := client.CreateProject(t, CreateProjectRequest{Name: "Child B", ParentID: &root.ID})
	client.UpdateProject(t, childB.ID, UpdateProjectRequest{StartedAt: &now})

	// Task in root with 3600s (1h) time entry
	rootTask := client.CreateTask(t, CreateTaskRequest{Name: "Root Task", ProjectID: &root.ID})
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     rootTask.ID,
		StartedAt:  time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		FinishedAt: timePtr(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)),
	})

	// Task in Child A with 1800s (30min) time entry
	taskA := client.CreateTask(t, CreateTaskRequest{Name: "Task A", ProjectID: &childA.ID})
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     taskA.ID,
		StartedAt:  time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		FinishedAt: timePtr(time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)),
	})

	// Task in Child B with 2700s (45min) finished + open entry (should not count)
	taskB := client.CreateTask(t, CreateTaskRequest{Name: "Task B", ProjectID: &childB.ID})
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     taskB.ID,
		StartedAt:  time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
		FinishedAt: timePtr(time.Date(2026, 3, 1, 11, 45, 0, 0, time.UTC)),
	})
	// Open entry on B's task (should NOT count)
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:    taskB.ID,
		StartedAt: time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC),
	})

	// Verify via GetProjectChildren
	result := client.GetProjectChildren(t, root.ID)

	// Root time_spent should be 3600 + 1800 + 2700 = 8100
	if result.Project.TimeSpent != 8100 {
		t.Errorf("root time_spent: want 8100, got %d", result.Project.TimeSpent)
	}

	// Find Child A and Child B in children
	for _, c := range result.Children {
		switch c.ID {
		case childA.ID:
			if c.TimeSpent != 1800 {
				t.Errorf("Child A time_spent: want 1800, got %d", c.TimeSpent)
			}
		case childB.ID:
			if c.TimeSpent != 2700 {
				t.Errorf("Child B time_spent: want 2700, got %d", c.TimeSpent)
			}
		}
	}
}

func TestE2E_ReorganizeWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Create 2 projects
	projA := client.CreateProject(t, CreateProjectRequest{Name: "Project A", Description: strPtr("Desc A")})
	client.UpdateProject(t, projA.ID, UpdateProjectRequest{StartedAt: &now})
	projB := client.CreateProject(t, CreateProjectRequest{Name: "Project B"})
	client.UpdateProject(t, projB.ID, UpdateProjectRequest{StartedAt: &now})

	// Create task in project A with a time entry (1h = 3600s)
	task := client.CreateTask(t, CreateTaskRequest{Name: "Movable Task", ProjectID: &projA.ID})
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     task.ID,
		StartedAt:  time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		FinishedAt: timePtr(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)),
	})

	// Move task to project B
	client.UpdateTask(t, task.ID, UpdateTaskRequest{ProjectID: &projB.ID})

	// Project A should have no tasks and time_spent == 0
	childrenA := client.GetProjectChildren(t, projA.ID)
	taskCount := 0
	for _, c := range childrenA.Children {
		if c.Type == "task" {
			taskCount++
		}
	}
	if taskCount != 0 {
		t.Errorf("Project A: expected 0 tasks after move, got %d", taskCount)
	}
	if childrenA.Project.TimeSpent != 0 {
		t.Errorf("Project A time_spent: want 0, got %d", childrenA.Project.TimeSpent)
	}

	// Project B should have the task with time_spent == 3600
	childrenB := client.GetProjectChildren(t, projB.ID)
	taskCount = 0
	for _, c := range childrenB.Children {
		if c.Type == "task" && c.ID == task.ID {
			taskCount++
		}
	}
	if taskCount != 1 {
		t.Errorf("Project B: expected 1 moved task, got %d", taskCount)
	}
	if childrenB.Project.TimeSpent != 3600 {
		t.Errorf("Project B time_spent: want 3600, got %d", childrenB.Project.TimeSpent)
	}

	// Create orphan task, verify in active tree at root
	orphan := client.CreateTask(t, CreateTaskRequest{Name: "Orphan"})
	client.UpdateTask(t, orphan.ID, UpdateTaskRequest{StartedAt: &now})

	tree := client.GetActiveTree(t)
	foundOrphan := false
	for _, n := range tree {
		if n.ID == orphan.ID && n.Type == "task" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Error("orphan task not found at root of active tree")
	}

	// Assign orphan to project A
	client.UpdateTask(t, orphan.ID, UpdateTaskRequest{ProjectID: &projA.ID})

	// Verify orphan moved into project A
	childrenA = client.GetProjectChildren(t, projA.ID)
	foundInA := false
	for _, c := range childrenA.Children {
		if c.ID == orphan.ID {
			foundInA = true
		}
	}
	if !foundInA {
		t.Error("orphan task not found in Project A after assignment")
	}

	// Partial update: change project name, verify description unchanged
	newName := "Project A Renamed"
	updated := client.UpdateProject(t, projA.ID, UpdateProjectRequest{Name: &newName})
	if updated.Name != newName {
		t.Errorf("name: want %q, got %q", newName, updated.Name)
	}
	if updated.Description == nil || *updated.Description != "Desc A" {
		t.Errorf("description should be unchanged (Desc A), got %v", updated.Description)
	}
}

func TestE2E_TimeEntryHistory_SplitsEntriesAtWeekBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	// Server is configured with Europe/Madrid (UTC+1 in February).
	// Create a time entry that crosses the weekly boundary (Sunday → Monday)
	// in Madrid time.
	//
	// Sunday 2026-02-22 23:00 Madrid = 22:00 UTC
	// Monday 2026-02-23 02:00 Madrid = 01:00 UTC
	// Total: 3 hours
	//
	// Expected split (Madrid weeks are Mon–Sun):
	//   Week of 2026-02-16: 1 hour  (Sun 23:00 → Mon 00:00 Madrid)
	//   Week of 2026-02-23: 2 hours (Mon 00:00 → Mon 02:00 Madrid)

	task := client.CreateTask(t, CreateTaskRequest{Name: "Cross-week task"})

	entryStart := time.Date(2026, 2, 22, 22, 0, 0, 0, time.UTC) // Sun 23:00 Madrid
	entryEnd := time.Date(2026, 2, 23, 1, 0, 0, 0, time.UTC)    // Mon 02:00 Madrid
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     task.ID,
		StartedAt:  entryStart,
		FinishedAt: &entryEnd,
	})

	history := client.GetTimeEntryHistory(t, "weekly", "2026-02-16", "2026-03-01")

	// Build a map of week start → hours for easy lookup
	weekHours := make(map[string]float32)
	for _, p := range history.Data {
		weekHours[p.Date] = p.Value
	}

	week1Hours := weekHours["2026-02-16"] // week containing Sunday Feb 22
	week2Hours := weekHours["2026-02-23"] // week starting Monday Feb 23

	// The entry should be split: 1h to week 1, 2h to week 2
	if week1Hours < 0.9 || week1Hours > 1.1 {
		t.Errorf("week of 2026-02-16: want ~1.0h, got %.2fh", week1Hours)
	}
	if week2Hours < 1.9 || week2Hours > 2.1 {
		t.Errorf("week of 2026-02-23: want ~2.0h, got %.2fh", week2Hours)
	}
}

func TestE2E_GetTasksByDueDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)
	taskDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	projectDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	laterTaskDue := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	// Create project with due date
	proj := client.CreateProject(t, CreateProjectRequest{
		Name:  "Due Date Project",
		DueAt: &projectDue,
	})
	client.UpdateProject(t, proj.ID, UpdateProjectRequest{StartedAt: &now})

	// Task 1: has own due_at (earliest) — should appear first
	task1 := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task with early due",
		ProjectID: &proj.ID,
		DueAt:     &taskDue,
	})

	// Task 2: has later due_at — should appear second
	task2 := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task with later due",
		ProjectID: &proj.ID,
		DueAt:     &laterTaskDue,
	})

	// Task 3: no own due_at, but project has due_at — should appear last (NULLS LAST)
	task3 := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task inheriting project due",
		ProjectID: &proj.ID,
	})

	// Task 4: finished task with due_at — should NOT appear
	finishedTaskDue := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	finishedTask := client.CreateTask(t, CreateTaskRequest{
		Name:      "Finished task",
		ProjectID: &proj.ID,
		DueAt:     &finishedTaskDue,
	})
	client.UpdateTask(t, finishedTask.ID, UpdateTaskRequest{
		StartedAt:  &now,
		FinishedAt: &now,
	})

	// Task 5: no due_at and no project — should NOT appear
	client.CreateTask(t, CreateTaskRequest{Name: "No due date anywhere"})

	// Add a time entry to task1 (1h = 3600s)
	client.CreateTimeEntry(t, CreateTimeEntryRequest{
		TaskID:     task1.ID,
		StartedAt:  time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		FinishedAt: timePtr(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)),
	})

	// Get tasks by due date
	tasks := client.GetTasksByDueDate(t)

	// Should contain exactly 3 tasks (task1, task2, task3)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify ordering: task1 (Jun 15) -> task2 (Sep 1) -> task3 (null task due, Dec 31 project due)
	if tasks[0].ID != task1.ID {
		t.Errorf("first task: want ID %d (earliest due), got ID %d", task1.ID, tasks[0].ID)
	}
	if tasks[1].ID != task2.ID {
		t.Errorf("second task: want ID %d (later due), got ID %d", task2.ID, tasks[1].ID)
	}
	if tasks[2].ID != task3.ID {
		t.Errorf("third task: want ID %d (null task due), got ID %d", task3.ID, tasks[2].ID)
	}

	// Verify task1 has time_spent
	if tasks[0].TimeSpent != 3600 {
		t.Errorf("task1 time_spent: want 3600, got %d", tasks[0].TimeSpent)
	}

	// Verify project info is populated
	if tasks[0].ProjectID == nil || *tasks[0].ProjectID != proj.ID {
		t.Errorf("task1 project_id: want %d, got %v", proj.ID, tasks[0].ProjectID)
	}
	if tasks[0].ProjectName == nil || *tasks[0].ProjectName != "Due Date Project" {
		t.Errorf("task1 project_name: want %q, got %v", "Due Date Project", tasks[0].ProjectName)
	}

	// Verify task3 has no own due_at but has project_due_at
	if tasks[2].DueAt != nil {
		t.Errorf("task3 due_at should be nil, got %v", tasks[2].DueAt)
	}
	if tasks[2].ProjectDueAt == nil {
		t.Error("task3 project_due_at should not be nil")
	}

	// Verify finished task is excluded
	for _, task := range tasks {
		if task.ID == finishedTask.ID {
			t.Error("finished task should not appear in results")
		}
	}
}

func TestE2E_TaskDependencies_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	// Create three tasks
	taskA := client.CreateTask(t, CreateTaskRequest{Name: "Task A"})
	taskB := client.CreateTask(t, CreateTaskRequest{Name: "Task B"})
	taskC := client.CreateTask(t, CreateTaskRequest{Name: "Task C"})

	// Create task D depending on A and B
	taskD := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task D",
		DependsOn: []int32{taskA.ID, taskB.ID},
	})

	// Verify create response has depends_on
	if len(taskD.DependsOn) != 2 {
		t.Fatalf("CreateTask D: expected 2 depends_on, got %d", len(taskD.DependsOn))
	}

	// Verify via GetTask(D): depends_on has A and B with names
	fullD := client.GetTask(t, taskD.ID)
	if len(fullD.DependsOn) != 2 {
		t.Fatalf("GetTask D: expected 2 depends_on, got %d", len(fullD.DependsOn))
	}
	depIDs := map[int32]string{}
	for _, dep := range fullD.DependsOn {
		depIDs[dep.ID] = dep.Name
	}
	if depIDs[taskA.ID] != "Task A" {
		t.Errorf("GetTask D: expected dep on Task A, got %v", depIDs)
	}
	if depIDs[taskB.ID] != "Task B" {
		t.Errorf("GetTask D: expected dep on Task B, got %v", depIDs)
	}

	// Verify reverse: GetTask(A) should show D in task_depends
	fullA := client.GetTask(t, taskA.ID)
	if len(fullA.TaskDepends) != 1 || fullA.TaskDepends[0].ID != taskD.ID {
		t.Errorf("GetTask A: expected task_depends [D=%d], got %v", taskD.ID, fullA.TaskDepends)
	}

	// Update D: change deps to [C] only
	updatedD := client.UpdateTask(t, taskD.ID, UpdateTaskRequest{
		DependsOn: int32SlicePtr([]int32{taskC.ID}),
	})
	if len(updatedD.DependsOn) != 1 || updatedD.DependsOn[0].ID != taskC.ID {
		t.Errorf("UpdateTask D to [C]: expected depends_on [C=%d], got %v", taskC.ID, updatedD.DependsOn)
	}

	// Verify A no longer has D in task_depends
	fullA = client.GetTask(t, taskA.ID)
	if len(fullA.TaskDepends) != 0 {
		t.Errorf("GetTask A after update: expected empty task_depends, got %v", fullA.TaskDepends)
	}

	// Update D: clear all deps
	updatedD = client.UpdateTask(t, taskD.ID, UpdateTaskRequest{
		DependsOn: int32SlicePtr([]int32{}),
	})
	if len(updatedD.DependsOn) != 0 {
		t.Errorf("UpdateTask D to []: expected empty depends_on, got %v", updatedD.DependsOn)
	}

	// Update D omitting depends_on: should stay empty
	updatedD = client.UpdateTask(t, taskD.ID, UpdateTaskRequest{
		Name: strPtr("Task D Renamed"),
	})
	if len(updatedD.DependsOn) != 0 {
		t.Errorf("UpdateTask D omitting deps: expected empty depends_on, got %v", updatedD.DependsOn)
	}
	if updatedD.Name != "Task D Renamed" {
		t.Errorf("UpdateTask D: expected name 'Task D Renamed', got %q", updatedD.Name)
	}
}

func TestE2E_TaskDependencies_ActiveTreeVisibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Create and start a project
	proj := client.CreateProject(t, CreateProjectRequest{Name: "Dep Project"})
	client.UpdateProject(t, proj.ID, UpdateProjectRequest{StartedAt: &now})

	// Create chain: A → B → C (C depends on B, B depends on A)
	taskA := client.CreateTask(t, CreateTaskRequest{Name: "Task A", ProjectID: &proj.ID})
	taskB := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task B",
		ProjectID: &proj.ID,
		DependsOn: []int32{taskA.ID},
	})
	taskC := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task C",
		ProjectID: &proj.ID,
		DependsOn: []int32{taskB.ID},
	})

	// Start all three
	client.UpdateTask(t, taskA.ID, UpdateTaskRequest{StartedAt: &now})
	client.UpdateTask(t, taskB.ID, UpdateTaskRequest{StartedAt: &now})
	client.UpdateTask(t, taskC.ID, UpdateTaskRequest{StartedAt: &now})

	// Helper to find task IDs in a project's children within the active tree
	findProjectChildren := func(tree []ActiveTreeNode, projID int32) []ActiveTreeNode {
		for _, n := range tree {
			if n.ID == projID && n.Type == "project" {
				return n.Children
			}
		}
		return nil
	}
	hasTask := func(children []ActiveTreeNode, taskID int32) bool {
		for _, c := range children {
			if c.ID == taskID && c.Type == "task" {
				return true
			}
		}
		return false
	}

	// --- Initial state ---
	// A: no deps → visible
	// B: depends on A (unfinished) → blocked, but A is not blocked → B not hidden → visible
	// C: depends on B → B is blocked (A unfinished) → all of C's deps blocked → C hidden
	tree := client.GetActiveTree(t)
	children := findProjectChildren(tree, proj.ID)

	if !hasTask(children, taskA.ID) {
		t.Error("initial: Task A should be visible in active tree")
	}
	if !hasTask(children, taskB.ID) {
		t.Error("initial: Task B should be visible (blocked but not hidden)")
	}
	if hasTask(children, taskC.ID) {
		t.Error("initial: Task C should be hidden (all deps blocked)")
	}

	// --- Finish A ---
	// B: A finished → B unblocked → visible
	// C: B unblocked → not all deps blocked → C visible
	client.UpdateTask(t, taskA.ID, UpdateTaskRequest{FinishedAt: &now})

	tree = client.GetActiveTree(t)
	children = findProjectChildren(tree, proj.ID)

	if hasTask(children, taskA.ID) {
		t.Error("after finishing A: Task A should not appear (finished)")
	}
	if !hasTask(children, taskB.ID) {
		t.Error("after finishing A: Task B should be visible (unblocked)")
	}
	if !hasTask(children, taskC.ID) {
		t.Error("after finishing A: Task C should now be visible")
	}

	// --- Finish B ---
	// C: B finished → C unblocked → visible
	client.UpdateTask(t, taskB.ID, UpdateTaskRequest{FinishedAt: &now})

	tree = client.GetActiveTree(t)
	children = findProjectChildren(tree, proj.ID)

	if hasTask(children, taskB.ID) {
		t.Error("after finishing B: Task B should not appear (finished)")
	}
	if !hasTask(children, taskC.ID) {
		t.Error("after finishing B: Task C should still be visible")
	}

	// --- Finish C ---
	client.UpdateTask(t, taskC.ID, UpdateTaskRequest{FinishedAt: &now})

	tree = client.GetActiveTree(t)
	children = findProjectChildren(tree, proj.ID)

	for _, c := range children {
		if c.Type == "task" {
			t.Errorf("after finishing all: no tasks should remain, found %s (id=%d)", c.Name, c.ID)
		}
	}
}

func TestE2E_TaskDependencies_DueDateList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	now := time.Now().UTC().Truncate(time.Second)
	dueDateA := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	dueDateC := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	// Task A: has due date June 15
	taskA := client.CreateTask(t, CreateTaskRequest{Name: "Task A", DueAt: &dueDateA})

	// Task B: no due date, depends on A → should inherit June 15
	taskB := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task B",
		DependsOn: []int32{taskA.ID},
	})

	// Task C: due Sept 1, depends on A → effective due = min(Sept 1, June 15) = June 15
	taskC := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task C",
		DueAt:     &dueDateC,
		DependsOn: []int32{taskA.ID},
	})

	tasks := client.GetTasksByDueDate(t)

	// All three should appear
	taskMap := map[int32]*TaskByDueDateResponse{}
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	if _, ok := taskMap[taskA.ID]; !ok {
		t.Fatal("Task A should appear in due-date list")
	}
	if _, ok := taskMap[taskB.ID]; !ok {
		t.Fatal("Task B should appear in due-date list (inherited due from A)")
	}
	if _, ok := taskMap[taskC.ID]; !ok {
		t.Fatal("Task C should appear in due-date list")
	}

	// B should have effective due date = June 15 (from A)
	if taskMap[taskB.ID].DueAt == nil {
		t.Fatal("Task B due_at should not be nil (inherited from dep A)")
	}
	if !taskMap[taskB.ID].DueAt.Equal(dueDateA) {
		t.Errorf("Task B effective due: want %v, got %v", dueDateA, *taskMap[taskB.ID].DueAt)
	}

	// C should have effective due date = June 15 (min of own Sept 1 and dep A's June 15)
	if taskMap[taskC.ID].DueAt == nil {
		t.Fatal("Task C due_at should not be nil")
	}
	if !taskMap[taskC.ID].DueAt.Equal(dueDateA) {
		t.Errorf("Task C effective due: want %v (from dep A), got %v", dueDateA, *taskMap[taskC.ID].DueAt)
	}

	// --- Hidden filtering with chain D → E → F ---
	dueDateD := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	taskD := client.CreateTask(t, CreateTaskRequest{Name: "Task D", DueAt: &dueDateD})
	client.UpdateTask(t, taskD.ID, UpdateTaskRequest{StartedAt: &now})

	taskE := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task E",
		DependsOn: []int32{taskD.ID},
	})
	client.UpdateTask(t, taskE.ID, UpdateTaskRequest{StartedAt: &now})

	taskF := client.CreateTask(t, CreateTaskRequest{
		Name:      "Task F",
		DependsOn: []int32{taskE.ID},
	})
	client.UpdateTask(t, taskF.ID, UpdateTaskRequest{StartedAt: &now})

	// F should be hidden: E is blocked (D unfinished), all of F's deps are blocked
	tasks = client.GetTasksByDueDate(t)
	taskMap = map[int32]*TaskByDueDateResponse{}
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	if _, ok := taskMap[taskD.ID]; !ok {
		t.Error("Task D should appear in due-date list")
	}
	if _, ok := taskMap[taskE.ID]; !ok {
		t.Error("Task E should appear in due-date list (blocked but not hidden)")
	}
	if _, ok := taskMap[taskF.ID]; ok {
		t.Error("Task F should be hidden from due-date list (all deps blocked)")
	}

	// Finish D → E unblocked → F should now appear
	client.UpdateTask(t, taskD.ID, UpdateTaskRequest{FinishedAt: &now})

	tasks = client.GetTasksByDueDate(t)
	taskMap = map[int32]*TaskByDueDateResponse{}
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	if _, ok := taskMap[taskF.ID]; !ok {
		t.Error("Task F should appear after finishing D (E no longer blocked)")
	}
}
