package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	createProjectFn func(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (CreateProjectResponse, error)
}

func (m *mockRepo) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (CreateProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, name, description, dueAt, parentID)
	}
	return CreateProjectResponse{}, nil
}

func TestService_CreateProject(t *testing.T) {
	now := time.Now()
	desc := "test description"
	desc2 := "read more books"
	parentID1 := int32(1)
	parentID5 := int32(5)

	tests := []struct {
		name    string
		req     CreateProjectRequest
		mockRes CreateProjectResponse
		mockErr error
		wantRes CreateProjectResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateProjectRequest{Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
			mockRes: CreateProjectResponse{ID: 1, Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
			wantRes: CreateProjectResponse{ID: 1, Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
		},
		{
			name:    "name only",
			req:     CreateProjectRequest{Name: "Minimal"},
			mockRes: CreateProjectResponse{ID: 2, Name: "Minimal"},
			wantRes: CreateProjectResponse{ID: 2, Name: "Minimal"},
		},
		{
			name:    "with description no due date",
			req:     CreateProjectRequest{Name: "Reading", Description: &desc2},
			mockRes: CreateProjectResponse{ID: 3, Name: "Reading", Description: &desc2},
			wantRes: CreateProjectResponse{ID: 3, Name: "Reading", Description: &desc2},
		},
		{
			name:    "with parent ID only",
			req:     CreateProjectRequest{Name: "Sub-project", ParentID: &parentID5},
			mockRes: CreateProjectResponse{ID: 4, Name: "Sub-project", ParentID: &parentID5},
			wantRes: CreateProjectResponse{ID: 4, Name: "Sub-project", ParentID: &parentID5},
		},
		{
			name:    "repository error",
			req:     CreateProjectRequest{Name: "Fail"},
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				createProjectFn: func(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (CreateProjectResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.CreateProject(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes.ID, got.ID)
			assert.Equal(t, tt.wantRes.Name, got.Name)
		})
	}
}
