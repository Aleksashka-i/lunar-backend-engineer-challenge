// Package httpapi exposes the rocket service over HTTP.
package httpapi

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"lunar-backend-engineer-challenge/internal/rocket"
)

//go:embed dashboard.html
var dashboard []byte

type handler struct {
	svc *rocket.Service
	log *slog.Logger
}

// New returns the HTTP handler for the rocket API.
func New(svc *rocket.Service, log *slog.Logger) http.Handler {
	h := &handler{svc: svc, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /messages", h.postMessage)
	mux.HandleFunc("GET /rockets", h.listRockets)
	mux.HandleFunc("GET /rockets/{channel}", h.getRocket)
	mux.HandleFunc("GET /{$}", h.getDashboard)
	return mux
}

func (h *handler) postMessage(w http.ResponseWriter, r *http.Request) {
	var m rocket.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		if !errors.Is(err, rocket.ErrInvalidMessage) { // malformed JSON
			err = fmt.Errorf("%w: %v", rocket.ErrInvalidMessage, err)
		}
		h.writeServiceError(w, err)
		return
	}
	if err := h.svc.ProcessMessage(r.Context(), m); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) listRockets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	field, err := rocket.ParseSortField(q.Get("sort"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var desc bool
	switch q.Get("order") {
	case "", "asc":
	case "desc":
		desc = true
	default:
		h.writeError(w, http.StatusBadRequest, `order must be "asc" or "desc"`)
		return
	}

	rockets, err := h.svc.List(r.Context(), field, desc)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, rockets)
}

func (h *handler) getRocket(w http.ResponseWriter, r *http.Request) {
	found, err := h.svc.Get(r.Context(), r.PathValue("channel"))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, found)
}

func (h *handler) getDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboard)
}

func (h *handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rocket.ErrInvalidMessage):
		h.writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, rocket.ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	default:
		h.log.Error("request failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Error("encode response", "error", err)
	}
}
