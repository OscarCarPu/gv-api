package backup

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"gv-api/internal/core"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	Run(ctx context.Context) (*Backup, error)
	List() ([]Backup, error)
	Path(name string) (string, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/backups", h.List)
	r.Post("/backups", h.Create)
	r.Get("/backups/{name}", h.Download)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	backups, err := h.service.List()
	if err != nil {
		core.InternalError(w, r, err, "Failed to list backups")
		return
	}
	core.JSON(w, http.StatusOK, backups)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	b, err := h.service.Run(r.Context())
	if errors.Is(err, ErrRunning) {
		core.Error(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		core.InternalError(w, r, err, "Backup failed")
		return
	}
	core.JSON(w, http.StatusCreated, b)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	path, err := h.service.Path(chi.URLParam(r, "name"))
	switch {
	case errors.Is(err, ErrInvalidName):
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, ErrNotFound):
		core.Error(w, http.StatusNotFound, err.Error())
		return
	case err != nil:
		core.InternalError(w, r, err, "Failed to read backup")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	http.ServeFile(w, r, path)
}
