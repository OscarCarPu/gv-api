package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/capacity"
	"gv-api/internal/history"
	"gv-api/internal/tasks"
	"gv-api/internal/tasks/mocks"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_CreateTask(t *testing.T) {
	t.Run("creates task with dependencies", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			CreateTask(mock.Anything, mock.Anything, "My Task", mock.Anything, mock.Anything, "standard", mock.Anything, int32(3), mock.Anything).
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
			CreateTask(mock.Anything, mock.Anything, "Simple Task", mock.Anything, mock.Anything, "standard", mock.Anything, int32(3), mock.Anything).
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
		repo.EXPECT().CreateTask(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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

	priorityCases := []struct {
		name  string
		input *int32
		want  int32
	}{
		{"defaults to 3 when not provided", nil, 3},
		{"uses provided priority", ptr(int32(1)), 1},
	}
	for _, tc := range priorityCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().
				CreateTask(mock.Anything, mock.Anything, "T", mock.Anything, mock.Anything, "standard", mock.Anything, tc.want, mock.Anything).
				Return(tasks.TaskResponse{ID: 1, Name: "T", Priority: tc.want}, nil)
			repo.EXPECT().GetTaskDependencies(mock.Anything, int32(1)).
				Return([]tasks.TaskDepRef{}, []tasks.TaskDepRef{}, false, nil)

			svc := tasks.NewService(repo, nil)
			got, err := svc.CreateTask(context.Background(), tasks.CreateTaskRequest{Name: "T", Priority: tc.input})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Priority)
		})
	}
}

func ptr[T any](v T) *T { return &v }

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
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, FinishedAt: tasks.NullableTime{Value: &now, Set: true}})
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
		_, err := svc.UpdateProject(context.Background(), tasks.UpdateProjectRequest{ID: 1, FinishedAt: tasks.NullableTime{Value: &now, Set: true}})
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
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, ProjectID: &projectID1, Name: "Task A", Description: &taskDesc, DueAt: &taskDue, Started: true, StartedAt: &taskStarted},
			{ID: 2, ProjectID: &projectID2, Name: "Task B"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
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

	t.Run("3-level deep project nesting", func(t *testing.T) {
		pid1 := int32(1)
		pid2 := int32(2)
		pid3 := int32(3)
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{
			{ID: 1, Name: "root"},
			{ID: 2, ParentID: &pid1, Name: "mid"},
			{ID: 3, ParentID: &pid2, Name: "leaf"},
		}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 10, ProjectID: &pid3, Name: "deep task", Started: true, StartedAt: &taskStarted},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
		require.NoError(t, err)

		// root → mid → leaf → deep task
		require.Len(t, got, 1)
		assert.Equal(t, "root", got[0].Name)
		require.Len(t, got[0].Children, 1)
		assert.Equal(t, "mid", got[0].Children[0].Name)
		require.Len(t, got[0].Children[0].Children, 1)
		assert.Equal(t, "leaf", got[0].Children[0].Children[0].Name)
		require.Len(t, got[0].Children[0].Children[0].Children, 1)
		assert.Equal(t, "deep task", got[0].Children[0].Children[0].Children[0].Name)
	})

	t.Run("orphan tasks at root level", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, Name: "Orphan Started", Started: true},
			{ID: 2, Name: "Orphan Unstarted"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, "Orphan Started", got[0].Name)
		assert.Equal(t, "Orphan Unstarted", got[1].Name)
	})

	t.Run("tasks with inactive project are excluded", func(t *testing.T) {
		inactiveProjectID := int32(99)
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 1, ProjectID: &inactiveProjectID, Name: "Task with inactive project", Started: true},
			{ID: 2, Name: "Root task"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Root task", got[0].Name)
	})

	t.Run("empty tree", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got)
	})

	t.Run("ordering: projects before started tasks before unstarted tasks", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{
			{ID: 1, Name: "Project"},
		}, nil)
		// Mock returns tasks in the order SQL would: started standard → unstarted standard
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: 4, ProjectID: &projectID1, Name: "Started Child", Started: true, TaskType: "standard"},
			{ID: 2, Name: "Started Orphan", Started: true, TaskType: "standard"},
			{ID: 3, ProjectID: &projectID1, Name: "Unstarted Child", TaskType: "standard"},
			{ID: 1, Name: "Unstarted Orphan", TaskType: "standard"},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
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
		_, err := svc.GetActiveTree(context.Background(), nil)
		assert.Error(t, err)
	})

	t.Run("error from GetUnfinishedTasks propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetActiveTree(context.Background(), nil)
		assert.Error(t, err)
	})

	t.Run("dependency fields are wired through to tree nodes", func(t *testing.T) {
		taskAID := int32(1)
		taskBID := int32(2)

		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, mock.Anything).Return([]tasks.UnfinishedTask{
			{ID: taskAID, Name: "Task A", Blocks: []tasks.TaskDepRef{{ID: taskBID, Name: "Task B"}}, Blocked: false},
			{ID: taskBID, Name: "Task B", DependsOn: []tasks.TaskDepRef{{ID: taskAID, Name: "Task A"}}, Blocked: true},
		}, nil)

		svc := tasks.NewService(repo, nil)
		got, err := svc.GetActiveTree(context.Background(), nil)
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
		assert.False(t, nodeA.Blocked)
		require.Len(t, nodeB.DependsOn, 1)
		assert.Equal(t, taskAID, nodeB.DependsOn[0].ID)
		assert.True(t, nodeB.Blocked)
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

	now := time.Now().UTC()
	expectedEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	expectedStart := expectedEnd.AddDate(0, -1, 0)

	repo.EXPECT().
		GetTimeEntryHistory(mock.Anything, "day", "UTC",
			mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedStart) }),
			mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedEnd) }),
		).
		Return([]history.Point{{Date: "2026-04-22", Value: 2.5}}, nil)

	svc := tasks.NewService(repo, loc)
	resp, err := svc.GetTimeEntryHistory(context.Background(), "daily", "", "")
	require.NoError(t, err)

	assert.Equal(t, expectedStart.Format("2006-01-02"), resp.StartAt)
	assert.Equal(t, expectedEnd.Format("2006-01-02"), resp.EndAt)
	require.Len(t, resp.Data, 1, "service is now a pass-through; repo decides fill")
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

		start := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
		end := time.Date(2026, 3, 31, 0, 0, 0, 0, loc)
		svc := tasks.NewService(repo, loc)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), start, &end)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, int32(1), result[0].ID)
		assert.Equal(t, "Task A", result[0].TaskName)
	})

	t.Run("defaults end_time to today when nil", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		loc := time.UTC
		now := time.Now().In(loc)
		expectedEnd := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)

		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"),
				mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedEnd) }),
			).
			Return([]tasks.TimeEntryWithTaskResponse{}, nil)

		start := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
		svc := tasks.NewService(repo, loc)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), start, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns empty slice when repo returns nil", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
			Return(nil, nil)

		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
		svc := tasks.NewService(repo, time.UTC)
		result, err := svc.GetTimeEntriesByDateRange(context.Background(), start, &end)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTimeEntriesByDateRange(mock.Anything, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
			Return(nil, errors.New("db error"))

		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
		svc := tasks.NewService(repo, time.UTC)
		_, err := svc.GetTimeEntriesByDateRange(context.Background(), start, &end)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})
}

func TestService_PriorityFilter(t *testing.T) {
	t.Run("GetActiveTree forwards min_priority to repo", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		threshold := int32(2)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, &threshold).Return([]tasks.UnfinishedTask{}, nil)

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetActiveTree(context.Background(), &threshold)
		require.NoError(t, err)
	})

	t.Run("GetActiveTree forwards nil when min_priority is unset", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetActiveProjects(mock.Anything).Return([]tasks.ActiveProject{}, nil)
		repo.EXPECT().GetUnfinishedTasks(mock.Anything, (*int32)(nil)).Return([]tasks.UnfinishedTask{}, nil)

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetActiveTree(context.Background(), nil)
		require.NoError(t, err)
	})

	t.Run("GetTasksByDueDate forwards min_priority to repo", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		threshold := int32(2)
		repo.EXPECT().GetTasksByDueDate(mock.Anything, &threshold).Return([]tasks.TaskByDueDateResponse{}, nil)

		svc := tasks.NewService(repo, nil)
		_, err := svc.GetTasksByDueDate(context.Background(), &threshold)
		require.NoError(t, err)
	})
}

type stubCapacityProvider struct {
	days []capacity.DayFreeBusy
}

func (s stubCapacityProvider) FreeBusyRange(_ context.Context, _, _ time.Time) ([]capacity.DayFreeBusy, error) {
	return s.days, nil
}

type stubPlannedHoursProvider struct{}

func (stubPlannedHoursProvider) PlannedHoursByTask(_ context.Context, _ []int32, _ time.Time) (map[int32]decimal.Decimal, error) {
	return map[int32]decimal.Decimal{}, nil
}

// due_at is stored as midnight UTC for its calendar date, while "today" is midnight in the
// server's own location (never UTC in the real deployment). A due date of "tomorrow" makes
// start_by fall on today's date for any task, regardless of its estimate — this asserts that
// both a task whose estimate fits inside today's free hours and one that doesn't still agree.
func TestService_GetTasksByDueDate_UrgencyAgreesAcrossOffset(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dueTomorrow := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, time.UTC)

	fitsToday := decimal.RequireFromString("1")
	exceedsToday := decimal.RequireFromString("25")

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		GetTasksByDueDate(mock.Anything, (*int32)(nil)).
		Return([]tasks.TaskByDueDateResponse{
			{ID: 1, TaskType: "standard", DueAt: &dueTomorrow, EstimateHours: &exceedsToday},
			{ID: 2, TaskType: "standard", DueAt: &dueTomorrow, EstimateHours: &fitsToday},
		}, nil)

	svc := tasks.NewService(repo, loc)
	svc.SetUrgencyProviders(
		stubCapacityProvider{days: []capacity.DayFreeBusy{
			{Date: today.Format("2006-01-02"), FreeHours: decimal.RequireFromString("14")},
		}},
		stubPlannedHoursProvider{},
	)

	got, err := svc.GetTasksByDueDate(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, r := range got {
		assert.Truef(t, r.Urgent, "task %d: start_by=%v urgent=%v (both must be urgent — start_by is today either way)", r.ID, r.StartBy, r.Urgent)
	}
}

// Two tasks sharing the same 5-day, 20-hour due-date window draw from the same freeByDate pool.
// Checked independently, a task needing only 3 hours would comfortably fit in the day furthest
// from today and not be urgent — but a higher-priority task claiming 16 of those 20 hours first
// (everything except today itself) pushes the second task's only remaining room onto today,
// which must show up as urgent rather than being silently missed because each task's own check
// looked fine in isolation.
func TestService_GetTasksByDueDate_UrgencyAccountsForCompetingTasks(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dueIn5Days := time.Date(today.Year(), today.Month(), today.Day()+5, 0, 0, 0, 0, time.UTC)

	days := make([]capacity.DayFreeBusy, 0, 5)
	for i := 0; i < 5; i++ {
		d := today.AddDate(0, 0, i)
		days = append(days, capacity.DayFreeBusy{Date: d.Format("2006-01-02"), FreeHours: decimal.RequireFromString("4")})
	}

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		GetTasksByDueDate(mock.Anything, (*int32)(nil)).
		Return([]tasks.TaskByDueDateResponse{
			{ID: 1, Priority: 1, TaskType: "standard", DueAt: &dueIn5Days, EstimateHours: decPtr("16")},
			{ID: 2, Priority: 3, TaskType: "standard", DueAt: &dueIn5Days, EstimateHours: decPtr("3")},
		}, nil)

	svc := tasks.NewService(repo, loc)
	svc.SetUrgencyProviders(
		stubCapacityProvider{days: days},
		stubPlannedHoursProvider{},
	)

	got, err := svc.GetTasksByDueDate(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, got, 2)

	byID := map[int32]tasks.TaskByDueDateResponse{}
	for _, r := range got {
		byID[r.ID] = r
	}

	assert.Falsef(t, byID[1].Urgent, "higher-priority task: start_by=%v urgent=%v", byID[1].StartBy, byID[1].Urgent)
	assert.Truef(t, byID[2].Urgent, "lower-priority task starved of shared hours must be urgent: start_by=%v urgent=%v", byID[2].StartBy, byID[2].Urgent)
}

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// A recurring task's time_spent accumulates across every past cycle (renewing only reschedules
// due_at). estimate_hours is a per-cycle target, so a task with a tiny estimate and a huge
// lifetime time_spent must not read as permanently "done" — the estimate should stay the
// remaining requirement every cycle, ignoring history.
func TestService_GetTasksByDueDate_RecurringUrgencyIgnoresLifetimeTimeSpent(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dueIn3Days := time.Date(today.Year(), today.Month(), today.Day()+3, 0, 0, 0, 0, time.UTC)

	days := make([]capacity.DayFreeBusy, 0, 3)
	for i := 0; i < 3; i++ {
		d := today.AddDate(0, 0, i)
		days = append(days, capacity.DayFreeBusy{Date: d.Format("2006-01-02"), FreeHours: decimal.RequireFromString("4")})
	}

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		GetTasksByDueDate(mock.Anything, (*int32)(nil)).
		Return([]tasks.TaskByDueDateResponse{
			{
				ID:            1,
				Priority:      3,
				TaskType:      "recurring",
				Recurrence:    ptr(int32(10)),
				DueAt:         &dueIn3Days,
				EstimateHours: decPtr("1.5"),
				TimeSpent:     30 * 3600, // 30h accumulated across past cycles
			},
		}, nil)

	svc := tasks.NewService(repo, loc)
	svc.SetUrgencyProviders(
		stubCapacityProvider{days: days},
		stubPlannedHoursProvider{},
	)

	got, err := svc.GetTasksByDueDate(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.NotNil(t, got[0].RemainingHours)
	assert.Truef(t, got[0].RemainingHours.Equal(decimal.RequireFromString("1.5")),
		"remaining_hours should be the per-cycle estimate untouched by lifetime time_spent, got %v", got[0].RemainingHours)
}

func TestService_GetTimeEntrySummary(t *testing.T) {
	loc := time.UTC
	now := time.Now().In(loc)
	expectedToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	daysSinceMonday := (int(now.Weekday()) + 6) % 7
	expectedWeek := time.Date(now.Year(), now.Month(), now.Day()-daysSinceMonday, 0, 0, 0, 0, loc)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		GetTimeEntrySummary(mock.Anything,
			mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedToday) }),
			mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedWeek) }),
		).
		Return(tasks.TimeEntrySummaryResponse{Today: 1800, Week: 7200}, nil)

	svc := tasks.NewService(repo, loc)
	got, err := svc.GetTimeEntrySummary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, tasks.WeeklyTaskTargetSeconds, got.WeeklyTargetSeconds)
}

func TestService_UpdateTask_ReplaceBlocks(t *testing.T) {
	blocks := []int32{5, 6}
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 1}, nil)
	repo.EXPECT().ReplaceTaskBlocks(mock.Anything, int32(1), blocks).Return(nil)
	repo.EXPECT().GetTaskDependencies(mock.Anything, int32(1)).
		Return([]tasks.TaskDepRef{}, []tasks.TaskDepRef{{ID: 5, Name: "A"}, {ID: 6, Name: "B"}}, false, nil)

	svc := tasks.NewService(repo, nil)
	got, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
		ID:     1,
		Blocks: &blocks,
	})
	require.NoError(t, err)
	require.Len(t, got.Blocks, 2)
	assert.Equal(t, int32(5), got.Blocks[0].ID)
}
