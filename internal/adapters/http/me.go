package httpadapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"meetopoly-be/internal/services/storage"
	tablesvc "meetopoly-be/internal/services/table"
	usersvc "meetopoly-be/internal/services/user"
)

type patchMeRequest struct {
	Username *string `json:"username"`
}

func handlePatchMe(users usersvc.Service, tables tablesvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		var req patchMeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Expected JSON body")
			return
		}
		if req.Username == nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "username is required")
			return
		}
		profile, err := users.UpdateUsername(r.Context(), userID, *req.Username)
		if err != nil {
			mapUserError(w, err)
			return
		}
		if tables != nil && profile.Username != "" {
			if err := tables.UpdateSeatedUsername(r.Context(), userID, profile.Username); err != nil {
				slog.Error("update seated username", "err", err, "userId", userID)
			}
		}
		writeJSON(w, http.StatusOK, toUserProfile(profile))
	}
}

func handleUploadAvatar(users usersvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		if err := r.ParseMultipartForm(storage.MaxAvatarBytes + (1 << 20)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Expected multipart form with file")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "file field is required")
			return
		}
		defer file.Close()

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		size := header.Size
		profile, err := users.UploadAvatar(r.Context(), userID, contentType, file, size)
		if err != nil {
			mapUserError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserProfile(profile))
	}
}

func handleDeleteAvatar(users usersvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
			return
		}
		profile, err := users.DeleteAvatar(r.Context(), userID)
		if err != nil {
			mapUserError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserProfile(profile))
	}
}

func mapUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usersvc.ErrInvalidUsername):
		writeError(w, http.StatusBadRequest, "invalid_username", "Username must be 3–20 characters: letters, digits, underscore")
	case errors.Is(err, usersvc.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "That username is already taken")
	case errors.Is(err, usersvc.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid session")
	case errors.Is(err, usersvc.ErrStorageNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "storage_not_configured", "Avatar storage is not configured")
	case errors.Is(err, usersvc.ErrInvalidImage):
		writeError(w, http.StatusBadRequest, "invalid_image", "Avatar must be JPEG, PNG, or WebP")
	case errors.Is(err, usersvc.ErrImageTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Avatar must be 1 MiB or smaller")
	default:
		slog.Error("user request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
