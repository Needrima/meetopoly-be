package user

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

const collectionName = "users"

// ErrNotFound is returned when no user matches.
var ErrNotFound = errors.New("user not found")

// ErrDuplicate is returned on unique index violation.
var ErrDuplicate = errors.New("user duplicate key")

type mongoDoc struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	Email         string             `bson:"email"`
	PasswordHash  string             `bson:"passwordHash,omitempty"`
	Username      string             `bson:"username,omitempty"`
	Country       string             `bson:"country,omitempty"`
	EmailVerified bool               `bson:"emailVerified"`
	CreatedAt     time.Time          `bson:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt"`
}

// MongoRepository implements Repository with MongoDB.
type MongoRepository struct {
	col *mongo.Collection
}

// NewMongoRepository binds to the users collection.
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
			Keys: bson.D{{Key: "username", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetSparse(true),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("user indexes: %w", err)
	}
	return nil
}

func (r *MongoRepository) Create(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now

	doc := mongoDoc{
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		Username:      u.Username,
		Country:       u.Country,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
	res, err := r.col.InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("user create: %w", err)
	}
	oid, ok := res.InsertedID.(primitive.ObjectID)
	if !ok {
		return fmt.Errorf("user create: unexpected id type %T", res.InsertedID)
	}
	u.ID = oid.Hex()
	return nil
}

func (r *MongoRepository) FindByID(ctx context.Context, id string) (*User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrNotFound
	}
	var doc mongoDoc
	err = r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user find by id: %w", err)
	}
	return docToUser(doc), nil
}

func (r *MongoRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var doc mongoDoc
	err := r.col.FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user find by email: %w", err)
	}
	return docToUser(doc), nil
}

func (r *MongoRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var doc mongoDoc
	err := r.col.FindOne(ctx, bson.M{"username": username}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user find by username: %w", err)
	}
	return docToUser(doc), nil
}

func (r *MongoRepository) Update(ctx context.Context, u *User) error {
	oid, err := primitive.ObjectIDFromHex(u.ID)
	if err != nil {
		return ErrNotFound
	}
	u.UpdatedAt = time.Now().UTC()
	update := bson.M{
		"$set": bson.M{
			"email":         u.Email,
			"passwordHash":  u.PasswordHash,
			"username":      u.Username,
			"country":       u.Country,
			"emailVerified": u.EmailVerified,
			"updatedAt":     u.UpdatedAt,
		},
	}
	res, err := r.col.UpdateByID(ctx, oid, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("user update: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func docToUser(doc mongoDoc) *User {
	return &User{
		ID:            doc.ID.Hex(),
		Email:         doc.Email,
		PasswordHash:  doc.PasswordHash,
		Username:      doc.Username,
		Country:       doc.Country,
		EmailVerified: doc.EmailVerified,
		CreatedAt:     doc.CreatedAt,
		UpdatedAt:     doc.UpdatedAt,
	}
}
