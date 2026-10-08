package httpadapter

import (
	"fmt"
	"net/http"

	locationsvc "meetopoly-be/internal/services/location"
)

// writeStaticJSON sets long-lived private cache headers for rarely changing catalog data.
func writeStaticJSON(w http.ResponseWriter, status int, v any) {
	maxAge := locationsvc.StaticContentCacheMaxAgeSec()
	w.Header().Set(
		"Cache-Control",
		fmt.Sprintf("private, max-age=%d, stale-while-revalidate=86400", maxAge),
	)
	writeJSON(w, status, v)
}
