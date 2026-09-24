package httpadapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	wsadapter "meetopoly-be/internal/adapters/websocket"
	"meetopoly-be/internal/services/auth"
	gamesvc "meetopoly-be/internal/services/game"
	"meetopoly-be/internal/services/health"
	locationsvc "meetopoly-be/internal/services/location"
	tablesvc "meetopoly-be/internal/services/table"
	usersvc "meetopoly-be/internal/services/user"
)

// Deps are HTTP adapter dependencies.
type Deps struct {
	Health     health.Service
	Auth       auth.Service
	Users      usersvc.Service
	Locations  locationsvc.Service
	Tables     tablesvc.Service
	Games      gamesvc.Service
	TableWS    *wsadapter.Hub
	GameWS     *wsadapter.GameHub
	PresenceWS *wsadapter.PresenceHub
}

// NewRouter builds the chi router for HTTP adapters.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsDev)

	r.Get("/health", handleHealth(deps.Health))

	r.Route("/auth", func(r chi.Router) {
		r.Post("/signup/start", handleSignupStart(deps.Auth))
		r.Post("/signup/verify", handleSignupVerify(deps.Auth))
		r.Post("/signup/password", handleSignupPassword(deps.Auth))
		r.Post("/signup/profile", handleSignupProfile(deps.Auth))
		r.Post("/login", handleLogin(deps.Auth))
		r.Post("/logout", handleLogout(deps.Auth))
		r.Get("/signup/status", handleSignupStatus(deps.Auth))
		r.Post("/password-reset/start", handlePasswordResetStart(deps.Auth))
		r.Post("/password-reset/verify", handlePasswordResetVerify(deps.Auth))
		r.Post("/password-reset/confirm", handlePasswordResetConfirm(deps.Auth))
	})

	r.Group(func(r chi.Router) {
		r.Use(requireSession(deps.Auth))
		r.Get("/me", handleMe(deps.Users))
		r.Get("/worlds", handleListWorlds(deps.Locations))
		r.Get("/locations", handleListLocations(deps.Locations))
		r.Get("/locations/by-slug", handleGetLocationBySlug(deps.Locations))
		r.Get("/locations/{locationId}", handleGetLocationByID(deps.Locations))

		r.Post("/tables/join", handleJoinTable(deps.Tables, deps.Users))
		r.Get("/tables/{tableId}", handleGetTable(deps.Tables))
		r.Post("/tables/{tableId}/ready", handleSetReady(deps.Tables))
		r.Post("/tables/{tableId}/leave", handleLeaveTable(deps.Tables))

		r.Get("/games/{gameId}", handleGetGame(deps.Games))
		r.Post("/games/{gameId}/roll", handleRollDice(deps.Games))
		r.Post("/games/{gameId}/end-turn", handleEndTurn(deps.Games))
		r.Post("/games/{gameId}/resign", handleResignGame(deps.Games))
		r.Post("/games/{gameId}/buy", handleBuyProperty(deps.Games))
		r.Post("/games/{gameId}/pin-color", handleSetPinColor(deps.Games))
	})

	if deps.TableWS != nil {
		r.Get("/ws/tables/{tableId}", deps.TableWS.HandleTable)
	}
	if deps.GameWS != nil {
		r.Get("/ws/games/{gameId}", deps.GameWS.HandleGame)
	}
	if deps.PresenceWS != nil {
		r.Get("/ws/presence/board/{gameId}", deps.PresenceWS.HandleBoardPresence)
	}

	return r
}


func handleHealth(svc health.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := svc.Check(r.Context())
		code := http.StatusOK
		if status.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, status)
	}
}

type signupStartRequest struct {
	Email string `json:"email"`
}

func handleSignupStart(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupStartRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		err := svc.StartSignup(r.Context(), req.Email)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type signupVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type signupVerifyResponse struct {
	SignupToken string `json:"signupToken"`
}

func handleSignupVerify(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupVerifyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		token, err := svc.VerifyEmail(r.Context(), req.Email, req.Code)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, signupVerifyResponse{SignupToken: token})
	}
}

type signupPasswordRequest struct {
	Password string `json:"password"`
}

func handleSignupPassword(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		var req signupPasswordRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		if err := svc.SetPassword(r.Context(), token, req.Password); err != nil {
			mapAuthError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type signupProfileRequest struct {
	Username string `json:"username"`
	Country  string `json:"country"`
}

func handleSignupProfile(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		var req signupProfileRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		sessionToken, profile, err := svc.CompleteProfile(r.Context(), token, req.Username, req.Country)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, authSessionResponse{
			Token: sessionToken,
			User:  toUserProfile(profile),
		})
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authSessionResponse struct {
	Token string      `json:"token"`
	User  userProfile `json:"user"`
}

type loginResponse struct {
	NeedsProfile bool        `json:"needsProfile"`
	Token        *string     `json:"token"`
	SignupToken  *string     `json:"signupToken"`
	User         userProfile `json:"user"`
}

func handleLogin(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		result, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		out := loginResponse{
			NeedsProfile: result.NeedsProfile,
			User:         toUserProfile(result.Profile),
		}
		if result.NeedsProfile {
			t := result.SignupToken
			out.SignupToken = &t
		} else {
			t := result.SessionToken
			out.Token = &t
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type signupStatusResponse struct {
	Email           string `json:"email"`
	PasswordSet     bool   `json:"passwordSet"`
	ProfileComplete bool   `json:"profileComplete"`
}

func handleSignupStatus(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		status, err := svc.SignupStatus(r.Context(), token)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, signupStatusResponse{
			Email:           status.Email,
			PasswordSet:     status.PasswordSet,
			ProfileComplete: status.ProfileComplete,
		})
	}
}

func handleLogout(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if err := svc.Logout(r.Context(), token); err != nil {
			mapAuthError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type passwordResetStartRequest struct {
	Email string `json:"email"`
}

type passwordResetStartResponse struct {
	Sent bool `json:"sent"`
}

func handlePasswordResetStart(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req passwordResetStartRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		sent, err := svc.StartPasswordReset(r.Context(), req.Email)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, passwordResetStartResponse{Sent: sent})
	}
}

type passwordResetVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type passwordResetVerifyResponse struct {
	ResetToken string `json:"resetToken"`
}

func handlePasswordResetVerify(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req passwordResetVerifyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		token, err := svc.VerifyPasswordReset(r.Context(), req.Email, req.Code)
		if err != nil {
			mapAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, passwordResetVerifyResponse{ResetToken: token})
	}
}

type passwordResetConfirmRequest struct {
	Password string `json:"password"`
}

func handlePasswordResetConfirm(svc auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		var req passwordResetConfirmRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
			return
		}
		if err := svc.ResetPassword(r.Context(), token, req.Password); err != nil {
			mapAuthError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMe(users usersvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		profile, err := users.GetByID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		writeJSON(w, http.StatusOK, toUserProfile(profile))
	}
}

type userProfile struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	Username        *string `json:"username"`
	Country         *string `json:"country"`
	EmailVerified   bool    `json:"emailVerified"`
	ProfileComplete bool    `json:"profileComplete"`
}

func toUserProfile(p *usersvc.Profile) userProfile {
	out := userProfile{
		ID:              p.ID,
		Email:           p.Email,
		EmailVerified:   p.EmailVerified,
		ProfileComplete: p.ProfileComplete,
	}
	if p.Username != "" {
		u := p.Username
		out.Username = &u
	}
	if p.Country != "" {
		c := p.Country
		out.Country = &c
	}
	return out
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func mapAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
	case errors.Is(err, auth.ErrInvalidCode):
		writeError(w, http.StatusUnauthorized, "invalid_code", "Invalid or expired verification code")
	case errors.Is(err, auth.ErrTooManyAttempts):
		writeError(w, http.StatusUnauthorized, "too_many_attempts", "Too many attempts; request a new code")
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "weak_password", "Password must be 8–128 chars with a letter and a digit")
	case errors.Is(err, auth.ErrInvalidUsername):
		writeError(w, http.StatusBadRequest, "invalid_username", "Username must be 3–20 characters: letters, digits, underscore")
	case errors.Is(err, auth.ErrInvalidCountry):
		writeError(w, http.StatusBadRequest, "invalid_country", "Country must be a 2-letter ISO code (e.g. NG)")
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "That username is already taken")
	case errors.Is(err, auth.ErrEmailUnverified):
		writeError(w, http.StatusUnauthorized, "email_unverified", "Verify your email before continuing")
	case errors.Is(err, auth.ErrIncompleteSignup):
		writeError(w, http.StatusUnauthorized, "incomplete_signup", "Finish signup before logging in")
	case errors.Is(err, auth.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials or session")
	default:
		slog.Error("auth request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}

// corsDev allows local Expo / web tooling to call the API.
func corsDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ResolveWSUser authenticates a WebSocket request via Bearer header or ?token=.
func ResolveWSUser(authSvc auth.Service) func(r *http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		token := bearerToken(r)
		if token == "" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		return authSvc.ResolveSession(r.Context(), token)
	}
}
