package location

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestLocationIndexModels(t *testing.T) {
	models := LocationIndexModels()
	if len(models) != 2 {
		t.Fatalf("len=%d want 2", len(models))
	}
	wantNames := map[string]bool{
		indexWorldBoard: false,
		indexWorldSlug:  false,
	}
	for _, m := range models {
		if m.Options == nil || m.Options.Name == nil {
			t.Fatal("index missing name")
		}
		name := *m.Options.Name
		if _, ok := wantNames[name]; !ok {
			t.Fatalf("unexpected index %q", name)
		}
		wantNames[name] = true
		if name == indexWorldSlug && (m.Options.Unique == nil || !*m.Options.Unique) {
			t.Fatal("slug index should be unique")
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("missing index %q", name)
		}
	}
}

func TestLocationIndexKeysListByWorld(t *testing.T) {
	models := LocationIndexModels()
	var boardKeys bson.D
	for _, m := range models {
		if m.Options != nil && m.Options.Name != nil && *m.Options.Name == indexWorldBoard {
			boardKeys = m.Keys.(bson.D)
			break
		}
	}
	if len(boardKeys) != 2 || boardKeys[0].Key != "worldId" || boardKeys[1].Key != "boardIndex" {
		t.Fatalf("board index keys=%v", boardKeys)
	}
}
