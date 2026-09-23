package httpadapter

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	gamesvc "meetopoly-be/internal/services/game"
)

func handleGetGame(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Get(r.Context(), gameID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func mapGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gamesvc.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Game not found")
	case errors.Is(err, gamesvc.ErrNeedPlayers):
		writeError(w, http.StatusBadRequest, "need_players", "Need at least 2 players to start")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
