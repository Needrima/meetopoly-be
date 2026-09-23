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

func handleRollDice(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Roll(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleEndTurn(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.EndTurn(r.Context(), gameID, userID)
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
	case errors.Is(err, gamesvc.ErrNotYourTurn):
		writeError(w, http.StatusConflict, "not_your_turn", "It is not your turn")
	case errors.Is(err, gamesvc.ErrNotPlayer):
		writeError(w, http.StatusForbidden, "not_player", "You are not in this game")
	case errors.Is(err, gamesvc.ErrInactive):
		writeError(w, http.StatusConflict, "inactive", "Game is not active")
	case errors.Is(err, gamesvc.ErrMustEndTurn):
		writeError(w, http.StatusConflict, "must_end_turn", "End your turn before rolling again")
	case errors.Is(err, gamesvc.ErrMustRoll):
		writeError(w, http.StatusConflict, "must_roll", "Roll before ending your turn")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
