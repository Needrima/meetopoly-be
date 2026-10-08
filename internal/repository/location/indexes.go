package location

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	indexWorldBoard = "worldId_1_boardIndex_1"
	indexWorldSlug  = "worldId_1_slug_1"
)

// LocationIndexModels returns Mongo indexes for the locations collection (Phase 23.5).
//
// explain() alignment (seed-scale ~3k docs):
//   - ListByWorldID {worldId} + sort boardIndex → IXSCAN worldId_1_boardIndex_1
//   - FindByWorldAndSlug {worldId, slug} → IXSCAN worldId_1_slug_1 (unique)
//   - FindByID {_id} → _id_
//   - ListWorlds $group on worldId → COLLSCAN (acceptable; served from Redis after warm)
func LocationIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "worldId", Value: 1}, {Key: "boardIndex", Value: 1}},
			Options: options.Index().SetName(indexWorldBoard),
		},
		{
			Keys:    bson.D{{Key: "worldId", Value: 1}, {Key: "slug", Value: 1}},
			Options: options.Index().SetUnique(true).SetName(indexWorldSlug),
		},
	}
}
