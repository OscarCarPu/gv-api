package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/history"
	"gv-api/internal/tasks"
	"gv-api/internal/tasks/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_CreateTask(t *testing.T) {
	t.Run("creates task with dependencies", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			CreateTask(mock.Anything, mock.Anything, "My Task", mock.Anything, mock.Anything, "standard", mock.Anything).
			Return(tasks.TaskResponse{ID: 10, Name: "My Task"}, nil)
		repo.EXPECT().
			ReplaceTaskDependencies(mock.Anything, int32(10), []int32{2, 3}).
			Return(nil)
		repo.EXPECT().
			GetTaskDependencies(mock.Anything, int32(10)).
			Return([]tasks.TaskDepRef{{ID: 2, Name: "A"}, {ID: 3, Name: "B"}}, []tasks.TaskDepRef{}, true, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.CreateTask(context.Background(), tasks.CreateTaskRequest{
			Name:      "My Task",
			DependsOn: []int32{2, 3},
		})
		require.NoError(t, err)
		assert.Equal(t, int32(10), got.ID)
		require.Len(t, got.DependsOn, 2)
		assert.Equal(t, int32(2), got.DependsOn[0].ID)
		assert.Empty(t, got.Blocks)
		assert.True(t, got.Blocked)
	})

	t.Run("creates task without dependencies", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			CreateTask(mock.Anything, mock.Anything, "Simple Task", mock.Anything, mock.Anything, "standard", mock.Anything).
			Return(tasks.TaskResponse{ID: 11, Name: "Simple Task"}, nil)
		repo.EXPECT().
			GetTaskDependencies(mock.Anything, int32(11)).
			Return([]tasks.TaskDepRef{}, []tasks.TaskDepRef{}, false, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.CreateTask(context.Background(), tasks.CreateTaskRequest{
			Name: "Simple Task",
		})
		require.NoError(t, err)
		assert.Equal(t, int32(11), got.ID)
		assert.Empty(t, got.DependsOn)
		assert.Empty(t, got.Blocks)
		assert.False(t, got.Blocked)
	})

	t.Run("propagates error from ReplaceTaskDependencies", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().CreateTask(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(tasks.TaskResponse{ID: 10}, nil)
		repo.EXPECT().ReplaceTaskDependencies(mock.Anything, int32(10), []int32{99}).
			Return(errors.New("fk violation"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.CreateTask(context.Background(), tasks.CreateTaskRequest{
			Name:      "T",
			DependsOn: []int32{99},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fk violation")
	})
}

func TestService_UpdateTask(t *testing.T) {
	t.Run("updates task with new dependencies", func(t *testing.T) {
		deps := []int32{5, 6}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
				return req.ID == 7
			})).
			Return(tasks.TaskResponse{ID: 7, Name: "T"}, nil)
		repo.EXPECT().
			ReplaceTaskDependencies(mock.Anything, int32(7), deps).
			Return(nil)
		repo.EXPECT().
			GetTaskDependencies(mock.Anything, int32(7)).
			Return([]tasks.TaskDepRef{{ID: 5, Name: "A"}, {ID: 6, Name: "B"}}, []tasks.TaskDepRef{{ID: 1, Name: "C"}}, true, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
			ID:        7,
			DependsOn: &deps,
		})
		require.NoError(t, err)
		require.Len(t, got.DependsOn, 2)
		assert.Equal(t, int32(5), got.DependsOn[0].ID)
		require.Len(t, got.Blocks, 1)
		assert.Equal(t, int32(1), got.Blocks[0].ID)
		assert.True(t, got.Blocked)
	})

	t.Run("clears dependencies with empty slice", func(t *testing.T) {
		empty := []int32{}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateTask(mock.Anything, mock.Anything).
			Return(tasks.TaskResponse{ID: 7, Name: "T"}, nil)
		repo.EXPECT().ReplaceTaskDependencies(mock.Anything, int32(7), empty).
			Return(nil)
		repo.EXPECT().GetTaskDependencies(mock.Anything, int32(7)).
			Return([]tasks.TaskDepRef{}, []tasks.TaskDepRef{}, false, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
			ID:        7,
			DependsOn: &empty,
		})
		require.NoError(t, err)
		assert.Empty(t, got.DependsOn)
		assert.False(t, got.Blocked)
	})

	t.Run("omitted depends_on does not call ReplaceTaskDependencies", func(t *testing.T) {
		name := "updated"
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateTask(mock.Anything, mock.Anything).
			Return(tasks.TaskResponse{ID: 7, Name: name}, nil)
		repo.EXPECT().GetTaskDependencies(mock.Anything, int32(7)).
			Return([]tasks.TaskDepRef{{ID: 2, Name: "A"}}, []tasks.TaskDepRef{}, true, nil)
		// ReplaceTaskDependencies should NOT be called

		svc := tasks.NewService(repo, nil)
		got, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
			ID:   7,
			Name: &name,
		})
		require.NoError(t, err)
		require.Len(t, got.DependsOn, 1)
		assert.Equal(t, int32(2), got.DependsOn[0].ID)
	})

	t.Run("propagates error from ReplaceTaskDependencies", func(t *testing.T) {
		deps := []int32{99}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateTask(mock.Anything, mock.Anything).
			Return(tasks.TaskResponse{ID: 7}, nil)
		repo.EXPECT().ReplaceTaskDependencies(mock.Anything, int32(7), deps).
			Return(errors.New("fk violation"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
			ID:        7,
			DependsOn: &deps,
		})
		require.Error(t, err)
	})
}

func TestService_UpdateProject(t *testing.T) {
	now := time.Now()
	name := "updated"

	t.Run("delegates to repo", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			UpdateProject(mock.Anything, mock.MatchedBy(func(req tasks.UpdateProjectRequest) bool {
				return req.ID == 1 && *req.Name == name
			})).
			Return(tasks.ProjectResponse{ID: 1, Name: name, FinishedAt: &now}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, Name: &name})
		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, name, got.Name)
	})

	t.Run("cascades finish to descendants when finished_at is set", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			UpdateProject(mock.Anything, mock.Anything).
			Return(tasks.ProjectResponse{ID: 1, Name: name, FinishedAt: &now}, nil)
		repo.EXPECT().FinishDescendantProjects(mock.Anything, int32(1)).Return(nil)
		repo.EXPECT().FinishTasksByProjectTree(mock.Anything, int32(1)).Return(nil)

		svc := tasks.NewService(repo, nil)
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, FinishedAt: &now})
		require.NoError(t, err)
	})

	t.Run("does not cascade when finished_at is not set", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			UpdateProject(mock.Anything, mock.Anything).
			Return(tasks.ProjectResponse{ID: 1, Name: name}, nil)

		svc := tasks.NewService(repo, nil)
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, Name: &name})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, errors.New("db error"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1})
		require.Error(t, err)
	})

	t.Run("propagates error from finish descendants", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{ID: 1, Name: "p"}, nil)
		repo.EXPECT().FinishDescendantProjects(mock.Anything, int32(1)).Return(errors.New("cascade error"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, FinishedAt: &now})
		require.Error(t, err)
	})
}

func TestService_GetActiveTree(t *testing.T) {
	parentID1 := int32(1)
	projectID1 := int32(1)
	projectID2 := int32(2)
	projDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	taskDesc := "important task"
	taskDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	taskStarted := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	t.Run("projects with nested sub-projects and tasks", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{
			{ID: 1, Name: "Parent Project", DueAt: &projDue},
			{ID: 2, ParentID: &parentID1, Name: "Child Project"},
		}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, ProjectID: &projectID1, Name: "Task A", Description: &taskDesc, DueAt: &taskDue, Started: true, StartedAt: &taskStarted},
			{ID: 2, ProjectID: &projectID2, Name: "Task B"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Parent Project", got[0].Name)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, &projDue, got[0].DueAt)

		require.Len(t, got[0].Children, 2)
		child := got[0].Children[0]
		assert.Equal(t, "Child Project", child.Name)
		assert.Equal(t, "project", child.Type)
		assert.Nil(t, child.DueAt)

		taskA := got[0].Children[1]
		assert.Equal(t, "Task A", taskA.Name)
		assert.Equal(t, "task", taskA.Type)
		assert.Equal(t, &taskDesc, taskA.Description)
		assert.Equal(t, &taskDue, taskA.DueAt)
		assert.Equal(t, &taskStarted, taskA.StartedAt)

		require.Len(t, got[0].Children[0].Children, 1)
		taskB := got[0].Children[0].Children[0]
		assert.Equal(t, "Task B", taskB.Name)
		assert.Nil(t, taskB.Description)
		assert.Nil(t, taskB.DueAt)
		assert.Nil(t, taskB.StartedAt)
	})

	t.Run("orphan tasks at root level", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, Name: "Orphan Started", Started: true},
			{ID: 2, Name: "Orphan Unstarted"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, "Orphan Started", got[0].Name)
		assert.Equal(t, "Orphan Unstarted", got[1].Name)
	})

	t.Run("tasks with inactive project are excluded from root", func(t *testing.T) {
		inactiveProjectID := int32(99)
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, ProjectID: &inactiveProjectID, Name: "Task with inactive project", Started: true},
			{ID: 2, Name: "Root task"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Root task", got[0].Name)
	})

	t.Run("empty tree", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got)
	})

	t.Run("ordering: projects before started tasks before unstarted tasks", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{
			{ID: 1, Name: "Project"},
		}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, Name: "Unstarted Orphan"},
			{ID: 2, Name: "Started Orphan", Started: true},
			{ID: 3, ProjectID: &projectID1, Name: "Unstarted Child"},
			{ID: 4, ProjectID: &projectID1, Name: "Started Child", Started: true},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 3)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, "Started Orphan", got[1].Name)
		assert.Equal(t, "Unstarted Orphan", got[2].Name)

		require.Len(t, got[0].Children, 2)
		assert.Equal(t, "Started Child", got[0].Children[0].Name)
		assert.Equal(t, "Unstarted Child", got[0].Children[1].Name)
	})

	t.Run("error from GetActiveProjects propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return(nil, errors.New("db error"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetActiveTree(context.Background())
		assert.Error(t, err)
	})

	t.Run("error from GetUnfinishedTasks propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return(nil, errors.New("db error"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetActiveTree(context.Background())
		assert.Error(t, err)
	})

	t.Run("dependency fields populated in tree nodes", func(t *testing.T) {
		taskAID := int32(1)
		taskBID := int32(2)

		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: taskAID, Name: "Task A", Blocks: []tasks.TaskDepRef{{ID: taskBID, Name: "Task B"}}},
			{ID: taskBID, Name: "Task B", DependsOn: []tasks.TaskDepRef{{ID: taskAID, Name: "Task A"}}},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)

		var nodeA, nodeB *tasks.ActiveTreeNode
		for i := range got {
			switch got[i].ID {
			case taskAID:
				nodeA = &got[i]
			case taskBID:
				nodeB = &got[i]
			}
		}

		require.NotNil(t, nodeA)
		require.NotNil(t, nodeB)
		require.Len(t, nodeA.Blocks, 1)
		assert.Equal(t, taskBID, nodeA.Blocks[0].ID)
		assert.Empty(t, nodeA.DependsOn)
		assert.False(t, nodeA.Blocked, "Task A has no deps, should not be blocked")
		require.Len(t, nodeB.DependsOn, 1)
		assert.Equal(t, taskAID, nodeB.DependsOn[0].ID)
		assert.Empty(t, nodeB.Blocks)
		assert.True(t, nodeB.Blocked, "Task B depends on unfinished A, should be blocked")
	})

	t.Run("task hidden when all dependencies are blocked", func(t *testing.T) {
		taskAID := int32(1)
		taskBID := int32(2)
		taskCID := int32(3)

		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: taskAID, Name: "Task A"},
			{ID: taskBID, Name: "Task B", DependsOn: []tasks.TaskDepRef{{ID: taskAID, Name: "Task A"}}},
			{ID: taskCID, Name: "Task C", DependsOn: []tasks.TaskDepRef{{ID: taskBID, Name: "Task B"}}},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		for _, node := range got {
			assert.NotEqual(t, taskCID, node.ID, "Task C should be hidden from active tree")
		}
		ids := make([]int32, len(got))
		for i, node := range got {
			ids[i] = node.ID
		}
		assert.Contains(t, ids, taskAID)
		assert.Contains(t, ids, taskBID)
	})

	t.Run("effective due date propagates backward through blocks", func(t *testing.T) {
		taskAID := int32(1)
		taskBID := int32(2)
		lateDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		earlyDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: taskAID, Name: "Task A", DueAt: &lateDue, Blocks: []tasks.TaskDepRef{{ID: taskBID, Name: "Task B"}}},
			{ID: taskBID, Name: "Task B", DueAt: &earlyDue, DependsOn: []tasks.TaskDepRef{{ID: taskAID, Name: "Task A"}}},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		var nodeA, nodeB *tasks.ActiveTreeNode
		for i := range got {
			switch got[i].ID {
			case taskAID:
				nodeA = &got[i]
			case taskBID:
				nodeB = &got[i]
			}
		}
		require.NotNil(t, nodeA)
		require.NotNil(t, nodeA.DueAt)
		assert.Equal(t, earlyDue, *nodeA.DueAt, "A should inherit B's earlier due date since A blocks B")

		require.NotNil(t, nodeB)
		require.NotNil(t, nodeB.DueAt)
		assert.Equal(t, earlyDue, *nodeB.DueAt, "B keeps its own due date")
	})
}

func TestService_GetTimeEntryHistory_InvalidFrequency(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := tasks.NewService(repo, time.UTC)

	_, err := svc.GetTimeEntryHistory(context.Background(), "yearly", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frequency")
}

func TestService_GetTimeEntryHistory_InvalidStartAt(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := tasks.NewService(repo, time.UTC)

	_, err := svc.GetTimeEntryHistory(context.Background(), "daily", "bad", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_at")
}

func TestService_GetTimeEntryHistory_InvalidEndAt(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := tasks.NewService(repo, time.UTC)

	_, err := svc.GetTimeEntryHistory(context.Background(), "daily", "", "bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid end_at")
}

func TestService_GetTimeEntryHistory_DefaultDatesDaily(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.UTC

	repo.EXPECT().
		GetTimeEntryHistory(mock.Anything, "day", "UTC", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return([]history.Point{
			{Date: "2026-03-17", Value: 2.5},
		}, nil)

	svc := tasks.NewService(repo, loc)
	resp, err := svc.GetTimeEntryHistory(context.Background(), "daily", "", "")
	require.NoError(t, err)

	assert.NotEmpty(t, resp.StartAt)
	assert.NotEmpty(t, resp.EndAt)
	// Should have filled data (at least 28 days for a daily default of 1 month)
	assert.True(t, len(resp.Data) >= 28, "expected at least 28 data points, got %d", len(resp.Data))
}

func TestService_GetTimeEntryHistory_FillsMissingPeriods(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	loc := time.UTC

	// Repo returns sparse data: only two points in a 5-day range
	repo.EXPECT().
		GetTimeEntryHistory(mock.Anything, "day", "UTC", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return([]history.Point{
			{Date: "2026-03-16", Value: 1},
			{Date: "2026-03-19", Value: 3},
		}, nil)

	svc := tasks.NewService(repo, loc)
	resp, err := svc.GetTimeEntryHistory(context.Background(), "daily", "2026-03-16", "2026-03-20")
	require.NoError(t, err)

	// 2026-03-16 through 2026-03-20 = 5 daily periods
	require.Len(t, resp.Data, 5)
	assert.Equal(t, float32(1), resp.Data[0].Value) // 03-16
	assert.Equal(t, float32(0), resp.Data[1].Value) // 03-17 filled
	assert.Equal(t, float32(0), resp.Data[2].Value) // 03-18 filled
	assert.Equal(t, float32(3), resp.Data[3].Value) // 03-19
	assert.Equal(t, float32(0), resp.Data[4].Value) // 03-20 filled
	assert.Equal(t, "2026-03-16", resp.StartAt)
	assert.Equal(t, "2026-03-20", resp.EndAt)
}

func TestService_GetTimeEntriesByDateRange(t *testing.T) {
	t.Run("parses dates and calls repo with timezone-aware timestamps", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		loc, _ := time.LoadLocation("Europe/Madrid")

		expectedStart := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
		expectedEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, loc) // end_time+1 day for inclusive

		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything,
				mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedStart) }),
				mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedEnd) }),
			).
			Return([]tasks.TimeEntryWithTaskResponse{
				{ID: 1, TaskID: 5, TaskName: "Task A", TimeSpent: 3600},
			}, nil)

		svc := tasks.NewService(repo, loc)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), "2026-03-01", "2026-03-31")
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, int32(1), result[0].ID)
		assert.Equal(t, "Task A", result[0].TaskName)
	})

	t.Run("defaults end_time to today when empty", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		loc := time.UTC
		now := time.Now().In(loc)
		expectedEnd := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)

		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"),
				mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedEnd) }),
			).
			Return([]tasks.TimeEntryWithTaskResponse{}, nil)

		svc := tasks.NewService(repo, loc)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), "2026-03-01", "")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns error for invalid start_time format", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := tasks.NewService(repo, time.UTC)

		_, err := svc.GetTimeEntriesByDateRange(context.Background(), "bad", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_time")
	})

	t.Run("returns error for invalid end_time format", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := tasks.NewService(repo, time.UTC)

		_, err := svc.GetTimeEntriesByDateRange(context.Background(), "2026-03-01", "bad")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end_time")
	})

	t.Run("returns empty slice when repo returns nil", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
			Return(nil, nil)

		svc := tasks.NewService(repo, time.UTC)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), "2026-03-01", "2026-03-31")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
			Return(nil, errors.New("db error"))

		svc := tasks.NewService(repo, time.UTC)
		_, err := svc.GetTimeEntriesByDateRange(context.Background(), "2026-03-01", "2026-03-31")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})
}
