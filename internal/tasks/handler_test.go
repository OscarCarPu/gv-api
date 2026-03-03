package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockService struct {
	createProjectFn func(ctx context.Context, req CreateProjectRequest) (CreateProjectResponse, error)
}

func (m *mockService) CreateProject(ctx context.Context, req CreateProjectRequest) (CreateProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, req)
	}
	return CreateProjectResponse{}, nil
}

// --- Handler Tests ---

func TestHandler_CreateProject(t *testing.T) {
	t.Run("returns 201 with created project", func(t *testing.T) {
		desc := "Test description"
		mock := &mockService{
			createProjectFn: func(ctx context.Context, req CreateProjectRequest) (CreateProjectResponse, error) {
				return CreateProjectResponse{ID: 1, Name: req.Name, Description: req.Description}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"name": "My Project", "description": "Test description"}`
		req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateProject(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got CreateProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "My Project", got.Name)
		assert.Equal(t, &desc, got.Description)
	})

	errorCases := []struct {
		name       string
		body       string
		setupMock  func() *mockService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 400 for invalid JSON",
			body:       "not json",
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid Body",
		},
		{
			name:       "returns 400 when name is missing",
			body:       `{"description": "no name provided"}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
		{
			name: "returns 500 when service fails",
			body: `{"name": "My Project"}`,
			setupMock: func() *mockService {
				return &mockService{
					createProjectFn: func(ctx context.Context, req CreateProjectRequest) (CreateProjectResponse, error) {
						return CreateProjectResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create project",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateProject(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}
