package game

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned when no game matches.
var ErrNotFound = errors.New("game not found")

// MongoRepository stores games in MongoDB.
type MongoRepository struct {
	col *mongo.Collection
}

// NewMongoRepository builds a games repository.
func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{col: db.Collection("games")}
}

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tableId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("game indexes: %w", err)
	}
	return nil
}

func (r *MongoRepository) Insert(ctx context.Context, g *Game) error {
	_, err := r.col.InsertOne(ctx, g)
	if err != nil {
		return fmt.Errorf("insert game: %w", err)
	}
	return nil
}

func (r *MongoRepository) Update(ctx context.Context, g *Game) error {
	g.UpdatedAt = time.Now().UTC()
	res, err := r.col.ReplaceOne(ctx, bson.M{"_id": g.ID}, g)
	if err != nil {
		return fmt.Errorf("update game: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) FindByID(ctx context.Context, id string) (*Game, error) {
	var g Game
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&g)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find game: %w", err)
	}
	return &g, nil
}

func (r *MongoRepository) FindByTableID(ctx context.Context, tableID string) (*Game, error) {
	var g Game
	err := r.col.FindOne(ctx, bson.M{"tableId": tableID}).Decode(&g)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find game by table: %w", err)
	}
	return &g, nil
}
