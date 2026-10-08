package httpadapter

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteStaticJSONCacheControl(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStaticJSON(rec, 200, map[string]string{"ok": "1"})

	cc := rec.Header().Get("Cache-Control")
	if cc == "" {
		t.Fatal("missing Cache-Control")
	}
	if !strings.Contains(cc, "private") {
		t.Fatalf("want private, got %q", cc)
	}
	if !strings.Contains(cc, "max-age=93600") {
		t.Fatalf("want 26h max-age, got %q", cc)
	}
	if !strings.Contains(cc, "stale-while-revalidate=86400") {
		t.Fatalf("want stale-while-revalidate, got %q", cc)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content-type=%q", rec.Header().Get("Content-Type"))
	}
}
