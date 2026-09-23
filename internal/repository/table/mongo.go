package table

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned when no table matches.
var ErrNotFound = errors.New("table not found")

// MongoRepository stores tables in MongoDB.
type MongoRepository struct {
	col *mongo.Collection
}

// NewMongoRepository builds a tables repository.
func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{col: db.Collection("tables")}
}

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "worldId", Value: 1},
				{Key: "status", Value: 1},
				{Key: "updatedAt", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "seats.userId", Value: 1}},
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("table indexes: %w", err)
	}
	return nil
}

func (r *MongoRepository) Insert(ctx context.Context, t *Table) error {
	_, err := r.col.InsertOne(ctx, t)
	if err != nil {
		return fmt.Errorf("insert table: %w", err)
	}
	return nil
}

func (r *MongoRepository) Update(ctx context.Context, t *Table) error {
	t.UpdatedAt = time.Now().UTC()
	res, err := r.col.ReplaceOne(ctx, bson.M{"_id": t.ID}, t)
	if err != nil {
		return fmt.Errorf("update table: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) FindByID(ctx context.Context, id string) (*Table, error) {
	var t Table
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find table: %w", err)
	}
	return &t, nil
}

func (r *MongoRepository) FindOpenLobby(ctx context.Context, worldID string) (*Table, error) {
	filter := bson.M{
		"worldId": worldID,
		"status":  StatusLobby,
	}
	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: 1}}).SetLimit(20)
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find open lobbies: %w", err)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var t Table
		if err := cur.Decode(&t); err != nil {
			return nil, fmt.Errorf("decode lobby: %w", err)
		}
		if OccupiedCount(&t) < MaxSeats {
			return &t, nil
		}
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate lobbies: %w", err)
	}
	return nil, ErrNotFound
}

func (r *MongoRepository) FindLobbyByUser(ctx context.Context, userID string) (*Table, error) {
	filter := bson.M{
		"status":       bson.M{"$in": []string{StatusLobby, StatusStarting}},
		"seats.userId": userID,
	}
	opts := options.FindOne().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
	var t Table
	err := r.col.FindOne(ctx, filter, opts).Decode(&t)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find lobby by user: %w", err)
	}
	return &t, nil
}

// OccupiedCount returns how many seats have a userId.
func OccupiedCount(t *Table) int {
	n := 0
	for _, s := range t.Seats {
		if s.UserID != "" {
			n++
		}
	}
	return n
}
