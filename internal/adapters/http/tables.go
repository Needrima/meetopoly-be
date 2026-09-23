package httpadapter

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	tablesvc "meetopoly-be/internal/services/table"
	usersvc "meetopoly-be/internal/services/user"
)

type joinTableRequest struct {
	WorldID string `json:"worldId"`
}

func handleJoinTable(tables tablesvc.Service, users usersvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req joinTableRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		profile, err := users.GetByID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		username := profile.Username
		if username == "" {
			username = profile.Email
		}
		view, err := tables.Join(r.Context(), userID, username, req.WorldID)
		if err != nil {
			mapTableError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleGetTable(tables tablesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tableID := chi.URLParam(r, "tableId")
		view, err := tables.Get(r.Context(), tableID)
		if err != nil {
			mapTableError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type setReadyRequest struct {
	Ready bool `json:"ready"`
}

func handleSetReady(tables tablesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		tableID := chi.URLParam(r, "tableId")
		var req setReadyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		view, err := tables.SetReady(r.Context(), tableID, userID, req.Ready)
		if err != nil {
			mapTableError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleLeaveTable(tables tablesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		tableID := chi.URLParam(r, "tableId")
		view, err := tables.Leave(r.Context(), tableID, userID)
		if err != nil {
			mapTableError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func mapTableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tablesvc.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Table not found")
	case errors.Is(err, tablesvc.ErrFull):
		writeError(w, http.StatusConflict, "table_full", "Table is full")
	case errors.Is(err, tablesvc.ErrNotSeated):
		writeError(w, http.StatusForbidden, "not_seated", "You are not seated at this table")
	case errors.Is(err, tablesvc.ErrNeedPlayers):
		writeError(w, http.StatusBadRequest, "need_players", "Need at least 2 players to Ready")
	case errors.Is(err, tablesvc.ErrWrongStatus):
		writeError(w, http.StatusConflict, "wrong_status", "Table is not in lobby")
	case errors.Is(err, tablesvc.ErrInvalidWorld):
		writeError(w, http.StatusBadRequest, "invalid_world", "worldId is required")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
