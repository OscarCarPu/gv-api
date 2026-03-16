package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/tasks"
	"gv-api/internal/tasks/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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
}
