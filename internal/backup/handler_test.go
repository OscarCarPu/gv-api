package backup_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gv-api/internal/backup"
	"gv-api/internal/backup/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func serve(svc backup.ServiceInterface, method, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	backup.NewHandler(svc).RegisterRoutes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHandler_List(t *testing.T) {
	t.Run("200 with the backups", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List().Return([]backup.Backup{{Name: "gv-db-20260101T000000Z.sql.gz", Size: 3}}, nil)

		rec := serve(svc, http.MethodGet, "/backups")

		assert.Equal(t, http.StatusOK, rec.Code)
		var body []backup.Backup
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "gv-db-20260101T000000Z.sql.gz", body[0].Name)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List().Return(nil, errors.New("disk gone"))

		rec := serve(svc, http.MethodGet, "/backups")

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Create(t *testing.T) {
	t.Run("201 with the new backup", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Run(mock.Anything).Return(&backup.Backup{Name: "gv-db-20260101T000000Z.sql.gz"}, nil)

		rec := serve(svc, http.MethodPost, "/backups")

		assert.Equal(t, http.StatusCreated, rec.Code)
		var body backup.Backup
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "gv-db-20260101T000000Z.sql.gz", body.Name)
	})

	t.Run("409 when a backup is already running", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Run(mock.Anything).Return(nil, backup.ErrRunning)

		rec := serve(svc, http.MethodPost, "/backups")

		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("500 when the dump fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Run(mock.Anything).Return(nil, errors.New("pg_dump failed"))

		rec := serve(svc, http.MethodPost, "/backups")

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Download(t *testing.T) {
	name := "gv-db-20260101T000000Z.sql.gz"

	t.Run("200 serves the file as an attachment", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(path, []byte("dump"), 0o600))
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Path(name).Return(path, nil)

		rec := serve(svc, http.MethodGet, "/backups/"+name)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/gzip", rec.Header().Get("Content-Type"))
		assert.Equal(t, `attachment; filename="`+name+`"`, rec.Header().Get("Content-Disposition"))
		assert.Equal(t, "dump", rec.Body.String())
	})

	t.Run("400 on an invalid name", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Path("notes.txt").Return("", backup.ErrInvalidName)

		rec := serve(svc, http.MethodGet, "/backups/notes.txt")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when the backup does not exist", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Path(name).Return("", backup.ErrNotFound)

		rec := serve(svc, http.MethodGet, "/backups/"+name)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
