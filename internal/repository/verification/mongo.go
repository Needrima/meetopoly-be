package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "email_verifications"

// ErrNotFound is returned when no verification exists for the email.
var ErrNotFound = errors.New("verification not found")

type mongoDoc struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Email     string             `bson:"email"`
	CodeHash  string             `bson:"codeHash"`
	ExpiresAt time.Time          `bson:"expiresAt"`
	Attempts  int                `bson:"attempts"`
	CreatedAt time.Time          `bson:"createdAt"`
}

// MongoRepository implements Repository with MongoDB.
type MongoRepository struct {
	col *mongo.Collection
}

// NewMongoRepository binds to email_verifications.
func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{col: db.Collection(collectionName)}
}

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("verification indexes: %w", err)
	}
	return nil
}

func (r *MongoRepository) Upsert(ctx context.Context, email, codeHash string, expiresAt time.Time) error {
	now := time.Now().UTC()
	filter := bson.M{"email": email}
	update := bson.M{
		"$set": bson.M{
			"codeHash":  codeHash,
			"expiresAt": expiresAt.UTC(),
			"attempts":  0,
			"createdAt": now,
		},
		"$setOnInsert": bson.M{
			"email": email,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.col.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("verification upsert: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindByEmail(ctx context.Context, email string) (*Record, error) {
	var doc mongoDoc
	err := r.col.FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("verification find: %w", err)
	}
	return &Record{
		ID:        doc.ID.Hex(),
		Email:     doc.Email,
		CodeHash:  doc.CodeHash,
		ExpiresAt: doc.ExpiresAt,
		Attempts:  doc.Attempts,
		CreatedAt: doc.CreatedAt,
	}, nil
}

func (r *MongoRepository) IncrementAttempts(ctx context.Context, email string) error {
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"email": email},
		bson.M{"$inc": bson.M{"attempts": 1}},
	)
	if err != nil {
		return fmt.Errorf("verification attempts: %w", err)
	}
	return nil
}

func (r *MongoRepository) DeleteByEmail(ctx context.Context, email string) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"email": email})
	if err != nil {
		return fmt.Errorf("verification delete: %w", err)
	}
	return nil
}
