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

func handleResignGame(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Resign(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleBuyProperty(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Buy(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type setPinColorRequest struct {
	PinColor string `json:"pinColor"`
}

func handleSetPinColor(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req setPinColorRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.SetPinColor(r.Context(), gameID, userID, req.PinColor)
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
	case errors.Is(err, gamesvc.ErrAlreadyOut):
		writeError(w, http.StatusConflict, "already_out", "You already resigned from this game")
	case errors.Is(err, gamesvc.ErrNotBuyable):
		writeError(w, http.StatusConflict, "not_buyable", "This space cannot be bought")
	case errors.Is(err, gamesvc.ErrAlreadyOwned):
		writeError(w, http.StatusConflict, "already_owned", "This space is already owned")
	case errors.Is(err, gamesvc.ErrCannotAfford):
		writeError(w, http.StatusConflict, "cannot_afford", "Not enough MeetCoin")
	case errors.Is(err, gamesvc.ErrMustSettle):
		writeError(w, http.StatusConflict, "must_settle", "Settle rent or tax before continuing (resign if you cannot pay)")
	case errors.Is(err, gamesvc.ErrInvalidPinColor):
		writeError(w, http.StatusBadRequest, "invalid_pin_color", "pinColor must be #RRGGBB")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
