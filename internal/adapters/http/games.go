package httpadapter

import (
	"encoding/json"
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

func handleBankruptGame(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Bankrupt(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleStartDebtPay(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.StartDebtPay(r.Context(), gameID, userID)
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

func handleStartAuction(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.StartAuction(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type auctionBidRequest struct {
	Amount int `json:"amount"`
}

func handleAuctionBid(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req auctionBidRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.AuctionBid(r.Context(), gameID, userID, req.Amount)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleAuctionFold(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.AuctionFold(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type buildRequest struct {
	BoardIndex int `json:"boardIndex"`
}

func handleBuild(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req buildRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Build(r.Context(), gameID, userID, req.BoardIndex)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleSellBuilding(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req buildRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.SellBuilding(r.Context(), gameID, userID, req.BoardIndex)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleMortgage(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req buildRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Mortgage(r.Context(), gameID, userID, req.BoardIndex)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleRedeem(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req buildRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.Redeem(r.Context(), gameID, userID, req.BoardIndex)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handlePayJailFine(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.PayJailFine(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleUseJailCard(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.UseJailCard(r.Context(), gameID, userID)
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

type enterHubRequest struct {
	HubID       string `json:"hubId"`
	HubRevision *int64 `json:"hubRevision,omitempty"`
}

func handleEnterHub(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req enterHubRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.EnterHub(r.Context(), gameID, userID, req.HubID, req.HubRevision)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleLeaveHub(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.LeaveHub(r.Context(), gameID, userID)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type tradeSideRequest struct {
	Cash             int   `json:"cash"`
	BoardIndexes     []int `json:"boardIndexes"`
	GetOutOfJailFree int   `json:"getOutOfJailFree"`
}

type tradeProposeRequest struct {
	ToUserID string            `json:"toUserId"`
	Give     tradeSideRequest  `json:"give"`
	Take     tradeSideRequest  `json:"take"`
}

func handleProposeTrade(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		var req tradeProposeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
		view, err := games.ProposeTrade(
			r.Context(),
			gameID,
			userID,
			req.ToUserID,
			gamesvc.TradeSideInput{
				Cash:             req.Give.Cash,
				BoardIndexes:     req.Give.BoardIndexes,
				GetOutOfJailFree: req.Give.GetOutOfJailFree,
			},
			gamesvc.TradeSideInput{
				Cash:             req.Take.Cash,
				BoardIndexes:     req.Take.BoardIndexes,
				GetOutOfJailFree: req.Take.GetOutOfJailFree,
			},
		)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type tradeAcceptRequest struct {
	MortgageAction string `json:"mortgageAction"`
}

func handleAcceptTrade(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		var req tradeAcceptRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		view, err := games.AcceptTrade(r.Context(), gameID, userID, req.MortgageAction)
		if err != nil {
			mapGameError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func handleDeclineTrade(games gamesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		gameID := chi.URLParam(r, "gameId")
		view, err := games.DeclineTrade(r.Context(), gameID, userID)
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
		writeError(w, http.StatusConflict, "must_settle", "Settle debt before continuing (sell, mortgage, or declare bankruptcy)")
	case errors.Is(err, gamesvc.ErrNotInDebt):
		writeError(w, http.StatusConflict, "not_in_debt", "You are not in debt")
	case errors.Is(err, gamesvc.ErrDebtPayActive):
		writeError(w, http.StatusConflict, "debt_pay_active", "Debt pay window is already open")
	case errors.Is(err, gamesvc.ErrNoDebtPay):
		writeError(w, http.StatusConflict, "no_debt_pay", "No debt pay window is open")
	case errors.Is(err, gamesvc.ErrMustResolveBuy), errors.Is(err, gamesvc.ErrMustBuy):
		writeError(w, http.StatusConflict, "must_resolve_buy", "Buy this property or start an auction before ending your turn")
	case errors.Is(err, gamesvc.ErrAuctionActive):
		writeError(w, http.StatusConflict, "auction_active", "Finish the auction first")
	case errors.Is(err, gamesvc.ErrNoAuction):
		writeError(w, http.StatusConflict, "no_auction", "No active auction")
	case errors.Is(err, gamesvc.ErrNoBuyOffer):
		writeError(w, http.StatusConflict, "no_buy_offer", "Nothing to auction")
	case errors.Is(err, gamesvc.ErrNotAuctionTurn):
		writeError(w, http.StatusConflict, "not_auction_turn", "It is not your turn to bid")
	case errors.Is(err, gamesvc.ErrBidTooLow):
		writeError(w, http.StatusConflict, "bid_too_low", "Bid must be at least the minimum")
	case errors.Is(err, gamesvc.ErrAlreadyFolded):
		writeError(w, http.StatusConflict, "already_folded", "You already folded from this auction")
	case errors.Is(err, gamesvc.ErrTradeActive):
		writeError(w, http.StatusConflict, "trade_active", "Resolve the open trade first")
	case errors.Is(err, gamesvc.ErrNoTrade):
		writeError(w, http.StatusConflict, "no_trade", "No open trade offer")
	case errors.Is(err, gamesvc.ErrNotTradeTarget):
		writeError(w, http.StatusForbidden, "not_trade_target", "Only the trade target can respond")
	case errors.Is(err, gamesvc.ErrInvalidTrade):
		writeError(w, http.StatusBadRequest, "invalid_trade", "Invalid trade offer")
	case errors.Is(err, gamesvc.ErrTradeNeedsDeed):
		writeError(w, http.StatusBadRequest, "trade_needs_deed", "Cash-for-cash trades are not allowed")
	case errors.Is(err, gamesvc.ErrTradeHasBuildings):
		writeError(w, http.StatusConflict, "trade_has_buildings", "Sell buildings before trading that property")
	case errors.Is(err, gamesvc.ErrTradeMortgageChoice):
		writeError(w, http.StatusBadRequest, "trade_mortgage_choice", "mortgageAction must be redeem_all or leave_all")
	case errors.Is(err, gamesvc.ErrInJail):
		writeError(w, http.StatusConflict, "in_jail", "You are in Jail")
	case errors.Is(err, gamesvc.ErrMustLeaveJail):
		writeError(w, http.StatusConflict, "must_leave_jail", "Pay the jail fine or use a Get Out of Jail Free card")
	case errors.Is(err, gamesvc.ErrNotInJail):
		writeError(w, http.StatusConflict, "not_in_jail", "You are not in Jail")
	case errors.Is(err, gamesvc.ErrNoJailCard):
		writeError(w, http.StatusConflict, "no_jail_card", "You have no Get Out of Jail Free card")
	case errors.Is(err, gamesvc.ErrInvalidPinColor):
		writeError(w, http.StatusBadRequest, "invalid_pin_color", "pinColor must be #RRGGBB")
	case errors.Is(err, gamesvc.ErrInvalidHubID):
		writeError(w, http.StatusBadRequest, "invalid_hub_id", "hubId must look like hub:{world}:{slug}")
	case errors.Is(err, gamesvc.ErrNotBuildable):
		writeError(w, http.StatusConflict, "not_buildable", "This space cannot be built on")
	case errors.Is(err, gamesvc.ErrNotOwner):
		writeError(w, http.StatusConflict, "not_owner", "You do not own this deed")
	case errors.Is(err, gamesvc.ErrNoMonopoly):
		writeError(w, http.StatusConflict, "no_monopoly", "Own the full color group before building")
	case errors.Is(err, gamesvc.ErrUnevenBuild):
		writeError(w, http.StatusConflict, "uneven_build", "Build evenly across the color group")
	case errors.Is(err, gamesvc.ErrUnevenSell):
		writeError(w, http.StatusConflict, "uneven_sell", "Sell evenly across the color group")
	case errors.Is(err, gamesvc.ErrNothingToSell):
		writeError(w, http.StatusConflict, "nothing_to_sell", "No building to sell on this deed")
	case errors.Is(err, gamesvc.ErrAlreadyMortgaged):
		writeError(w, http.StatusConflict, "already_mortgaged", "Deed is already mortgaged")
	case errors.Is(err, gamesvc.ErrNotMortgaged):
		writeError(w, http.StatusConflict, "not_mortgaged", "Deed is not mortgaged")
	case errors.Is(err, gamesvc.ErrMustSellBuildings):
		writeError(w, http.StatusConflict, "must_sell_buildings", "Sell all buildings on the color group before mortgaging")
	case errors.Is(err, gamesvc.ErrCannotMortgage):
		writeError(w, http.StatusConflict, "cannot_mortgage", "This space cannot be mortgaged")
	case errors.Is(err, gamesvc.ErrMaxBuilt):
		writeError(w, http.StatusConflict, "max_built", "Hotel already built on this deed")
	case errors.Is(err, gamesvc.ErrMortgagedSet):
		writeError(w, http.StatusConflict, "mortgaged_set", "Cannot build while a deed in the set is mortgaged")
	case errors.Is(err, gamesvc.ErrInvalidBoardIndex):
		writeError(w, http.StatusBadRequest, "invalid_board_index", "Invalid boardIndex")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
