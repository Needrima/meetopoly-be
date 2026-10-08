package location

import "testing"

func TestStaticContentCacheMaxAgeSec(t *testing.T) {
	if got := StaticContentCacheMaxAgeSec(); got != 93600 {
		t.Fatalf("max-age=%d want 93600 (26h)", got)
	}
}
