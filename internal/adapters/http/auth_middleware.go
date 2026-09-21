package httpadapter

import (
	"context"
	"net/http"

	"meetopoly-be/internal/services/auth"
)

const userIDContextKey = "userID"

func requireSession(authSvc auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			userID, err := authSvc.ResolveSession(r.Context(), token)
			if err != nil {
				mapAuthError(w, err)
				return
			}
			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func userIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDContextKey).(string)
	return id, ok && id != ""
}
