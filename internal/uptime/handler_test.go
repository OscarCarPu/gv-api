package uptime_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gv-api/internal/uptime"
	"gv-api/internal/uptime/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Handler tests cover HTTP concerns only: query parsing, error to status mapping, and the
// one status that is a deployment state rather than a fault (503 when no pipeline database
// is wired up).

func errMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body["error"]
}

func TestHandler_Overview(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Overview(mock.Anything).Return(uptime.Overview{Stale: true}, nil)

		rec := httptest.NewRecorder()
		uptime.NewHandler(svc).Overview(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("503 when the pipeline database is not configured", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Overview(mock.Anything).Return(uptime.Overview{}, uptime.ErrNotConfigured)

		rec := httptest.NewRecorder()
		uptime.NewHandler(svc).Overview(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, errMessage(t, rec), "not configured")
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Overview(mock.Anything).Return(uptime.Overview{}, errors.New("db down"))

		rec := httptest.NewRecorder()
		uptime.NewHandler(svc).Overview(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Windows_ParsesQuery(t *testing.T) {
	var got uptime.WindowsQuery
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().Windows(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, q uptime.WindowsQuery) (uptime.WindowsReport, error) {
			got = q
			return uptime.WindowsReport{}, nil
		})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/?device=watchdog&from=2026-01-01T00:00:00Z&to=2026-02-01&limit=50", nil)
	uptime.NewHandler(svc).Windows(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, got.Device)
	assert.Equal(t, uptime.DeviceWatchdog, *got.Device)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), got.From)
	// A bare date is accepted as UTC midnight, which is what the pipeline's timestamps are in.
	assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), got.To)
	assert.Equal(t, 50, got.Limit)
}

func TestHandler_Windows_NoQueryLeavesDefaultsToService(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().Windows(mock.Anything, uptime.WindowsQuery{}).Return(uptime.WindowsReport{}, nil)

	rec := httptest.NewRecorder()
	uptime.NewHandler(svc).Windows(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_Windows_BadRequests(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"unknown device", "?device=printer", "device must be one of lab, watchdog"},
		{"unparseable from", "?from=yesterday", "invalid from"},
		{"unparseable to", "?to=13/06/2026", "invalid to"},
		{"zero limit", "?limit=0", "limit must be a positive integer"},
		{"non-numeric limit", "?limit=all", "limit must be a positive integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The service is never reached, so an unconfigured mock is the assertion.
			svc := mocks.NewMockServiceInterface(t)
			rec := httptest.NewRecorder()
			uptime.NewHandler(svc).Windows(rec, httptest.NewRequest(http.MethodGet, "/"+tc.query, nil))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, errMessage(t, rec), tc.want)
		})
	}
}

func TestHandler_Windows_InvalidRangeIs400(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().Windows(mock.Anything, mock.Anything).Return(uptime.WindowsReport{}, uptime.ErrInvalidRange)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?from=2026-02-01&to=2026-01-01", nil)
	uptime.NewHandler(svc).Windows(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, errMessage(t, rec), "from must be before to")
}

func TestHandler_Windows_NotConfiguredIs503(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().Windows(mock.Anything, mock.Anything).Return(uptime.WindowsReport{}, uptime.ErrNotConfigured)

	rec := httptest.NewRecorder()
	uptime.NewHandler(svc).Windows(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
