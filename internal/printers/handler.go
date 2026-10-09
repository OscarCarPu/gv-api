package printers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"gv-api/internal/core"

	"github.com/go-chi/chi/v5"
)

const (
	uploadDeadline = 15 * time.Minute
	maxChunkBytes  = 100 << 20
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/domotics/printers", h.List)
	r.Get("/domotics/printers/{id}/status", h.Status)
	r.Delete("/domotics/printers/{id}/job", h.StopJob)
	r.Get("/domotics/printers/{id}/files", h.Files)
	r.Put("/domotics/printers/{id}/files", h.Upload)
	r.Patch("/domotics/printers/{id}/files", h.UploadChunk)
	r.Post("/domotics/printers/{id}/files", h.StartPrint)
	r.Delete("/domotics/printers/{id}/files", h.DeleteFile)
	r.Get("/domotics/printers/{id}/files/progress", h.Progress)
	r.Get("/domotics/printers/{id}/camera", h.Camera)
	r.Get("/domotics/printers/{id}/recordings", h.Recordings)
	r.Post("/domotics/printers/{id}/recordings", h.RecordingAction)
	r.Delete("/domotics/printers/{id}/recordings", h.DeleteRecording)
}

func (h *Handler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/domotics/printers/{id}/recordings/{name}", h.Media)
}

func (h *Handler) unit(w http.ResponseWriter, r *http.Request) (*unit, bool) {
	un, ok := h.service.unit(chi.URLParam(r, "id"))
	if !ok {
		core.Error(w, http.StatusNotFound, "Printer not found")
	}
	return un, ok
}

func (h *Handler) configured(w http.ResponseWriter, r *http.Request) (*unit, bool) {
	un, ok := h.unit(w, r)
	if ok && un.p.Host == "" {
		core.Error(w, http.StatusServiceUnavailable, "PrusaLink not configured")
		return nil, false
	}
	return un, ok
}

func failStatus(w http.ResponseWriter, err error, fallback string) {
	var se *statusError
	if errors.As(err, &se) {
		core.Error(w, se.status, se.msg)
		return
	}
	msg := fallback
	if err != nil {
		msg = err.Error()
	}
	core.Error(w, http.StatusBadGateway, msg)
}

func upstreamStatus(status int) int {
	if status == http.StatusUnauthorized {
		return http.StatusBadGateway
	}
	return status
}

func (h *Handler) List(w http.ResponseWriter, _ *http.Request) {
	core.JSON(w, http.StatusOK, h.service.List())
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	if un, ok := h.unit(w, r); ok {
		core.JSON(w, http.StatusOK, un.link.status(r.Context()))
	}
}

func (h *Handler) StopJob(w http.ResponseWriter, r *http.Request) {
	un, ok := h.configured(w, r)
	if !ok {
		return
	}
	outcome, status, err := un.link.stopJob(r.Context())
	switch {
	case err != nil:
		failStatus(w, err, "Could not stop the print")
	case outcome == stopIdle:
		core.Error(w, http.StatusConflict, "No print is running")
	case outcome == stopRejected:
		core.Error(w, upstreamStatus(status), stopMessage(status))
	default:
		core.JSON(w, http.StatusOK, map[string]bool{"stopped": true})
	}
}

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	if un, ok := h.unit(w, r); ok {
		core.JSON(w, http.StatusOK, un.link.files(r.Context()))
	}
}

func (h *Handler) uploadTarget(w http.ResponseWriter, r *http.Request) (*unit, string, bool) {
	un, ok := h.configured(w, r)
	if !ok {
		return nil, "", false
	}
	raw := r.Header.Get("X-File-Name")
	if raw == "" {
		core.Error(w, http.StatusBadRequest, "Missing X-File-Name header")
		return nil, "", false
	}
	name := sanitizeFileName(unescapeHeader(raw))
	if name == "" {
		core.Error(w, http.StatusBadRequest, "Unsupported file name. Expected a .bgcode, .gcode, .gco or .g file.")
		return nil, "", false
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadDeadline))
	return un, name, true
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	un, name, ok := h.uploadTarget(w, r)
	if !ok {
		return
	}
	id := r.Header.Get("X-Upload-Id")
	if !uploadIDPattern.MatchString(id) {
		id = newUploadID()
	}

	n, err := h.service.uploads.stageWhole(id, http.MaxBytesReader(w, r.Body, maxUploadBytes))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "Could not read the uploaded file")
		return
	}
	if n == 0 {
		_ = os.Remove(h.service.uploads.path(id))
		core.Error(w, http.StatusBadRequest, "Empty file")
		return
	}
	if err := h.service.forward(un, id, name, r.URL.Query().Get("overwrite") == "1"); err != nil {
		core.InternalError(w, r, err, "Could not stage the uploaded file")
		return
	}
	core.JSON(w, http.StatusAccepted, map[string]any{"name": name, "uploadId": id})
}

func (h *Handler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	un, name, ok := h.uploadTarget(w, r)
	if !ok {
		return
	}
	id := r.Header.Get("X-Upload-Id")
	offset, okOffset := parseByteHeader(r.Header.Get("X-Upload-Offset"))
	total, okTotal := parseByteHeader(r.Header.Get("X-Upload-Total"))
	if !uploadIDPattern.MatchString(id) || !okOffset || !okTotal {
		core.Error(w, http.StatusBadRequest, "Missing or malformed X-Upload-Id / X-Upload-Offset / X-Upload-Total header")
		return
	}

	chunk, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBytes))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "Could not read the uploaded file")
		return
	}

	u := h.service.uploads
	if offset == 0 {
		u.sweepStaged()
	}
	received, complete, err := u.appendChunk(id, offset, total, chunk)
	var ce *chunkError
	switch {
	case errors.As(err, &ce):
		core.JSON(w, ce.status, map[string]any{"error": ce.msg, "received": ce.received})
		return
	case err != nil:
		core.Error(w, http.StatusInternalServerError, "Could not stage the uploaded chunk")
		return
	case !complete:
		core.JSON(w, http.StatusOK, map[string]any{"received": received, "complete": false})
		return
	}

	if err := h.service.forward(un, id, name, r.URL.Query().Get("overwrite") == "1"); err != nil {
		core.InternalError(w, r, err, "Could not stage the uploaded file")
		return
	}
	core.JSON(w, http.StatusAccepted, map[string]any{"name": name, "uploadId": id, "received": received, "complete": true})
}

func (h *Handler) fileCommand(w http.ResponseWriter, r *http.Request, method, op, failure string, extra map[string]string) {
	un, ok := h.unit(w, r)
	if !ok {
		return
	}
	name := sanitizeFileName(r.URL.Query().Get("name"))
	if name == "" {
		core.Error(w, http.StatusBadRequest, "Missing or invalid file name")
		return
	}
	status, err := un.link.command(r.Context(), method, name, extra)
	switch {
	case err != nil:
		failStatus(w, err, failure)
	case status < 200 || status > 299:
		core.Error(w, upstreamStatus(status), upstreamMessage(status, op))
	default:
		core.JSON(w, http.StatusOK, map[string]string{"name": name})
	}
}

func (h *Handler) StartPrint(w http.ResponseWriter, r *http.Request) {
	h.fileCommand(w, r, http.MethodPost, "print", "Could not start the print", nil)
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	h.fileCommand(w, r, http.MethodDelete, "delete", "Could not delete the file", map[string]string{"Force": "?0"})
}

func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	u := h.service.uploads
	raw, has := r.URL.Query()["u"]
	if !has {
		core.JSON(w, http.StatusOK, map[string]any{"uploads": u.list(chi.URLParam(r, "id"))})
		return
	}
	if !uploadIDPattern.MatchString(raw[0]) {
		core.Error(w, http.StatusBadRequest, "Invalid upload id")
		return
	}
	core.JSON(w, http.StatusOK, u.get(raw[0]))
}

func (h *Handler) Camera(w http.ResponseWriter, r *http.Request) {
	un, ok := h.unit(w, r)
	if !ok {
		return
	}
	frame, err := un.cam.frame(r.Context())
	if err != nil {
		core.Error(w, http.StatusServiceUnavailable, "No frame available")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(frame)
}

func (h *Handler) Recordings(w http.ResponseWriter, r *http.Request) {
	if un, ok := h.unit(w, r); ok {
		core.JSON(w, http.StatusOK, h.service.recordings(un))
	}
}

func (h *Handler) RecordingAction(w http.ResponseWriter, r *http.Request) {
	un, ok := h.unit(w, r)
	if !ok {
		return
	}
	switch r.URL.Query().Get("action") {
	case "start":
		rec, err := un.rec.start()
		if err != nil {
			failStatus(w, err, "Could not start recording")
			return
		}
		core.JSON(w, http.StatusCreated, rec)
	case "stop":
		if err := un.rec.stop(); err != nil {
			failStatus(w, err, "Could not stop recording")
			return
		}
		core.JSON(w, http.StatusOK, h.service.recordings(un))
	default:
		core.Error(w, http.StatusBadRequest, "Unknown action. Expected start or stop.")
	}
}

func (h *Handler) DeleteRecording(w http.ResponseWriter, r *http.Request) {
	un, ok := h.unit(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if err := un.rec.remove(name); err != nil {
		failStatus(w, err, "Could not delete the recording")
		return
	}
	core.JSON(w, http.StatusOK, map[string]string{"name": name})
}

func (h *Handler) Media(w http.ResponseWriter, r *http.Request) {
	id, name := chi.URLParam(r, "id"), chi.URLParam(r, "name")
	q := r.URL.Query()
	if !h.service.verify(id, name, q.Get("exp"), q.Get("sig")) {
		core.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	un, ok := h.unit(w, r)
	if !ok {
		return
	}
	f, st, ok := un.rec.open(name)
	if !ok {
		core.Error(w, http.StatusNotFound, "Recording not found")
		return
	}
	defer func() { _ = f.Close() }()

	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	ctype := "video/mp4"
	if name[len(name)-4:] == ".jpg" {
		ctype = "image/jpeg"
	}
	w.Header().Set("Content-Type", ctype)
	if un.rec.liveName() == name {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	}
	if q.Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+"_"+name+`"`)
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}
