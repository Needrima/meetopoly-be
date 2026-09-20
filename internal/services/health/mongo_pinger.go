package health

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// MongoPinger adapts *mongo.Client to health.Pinger.
type MongoPinger struct {
	Client *mongo.Client
}

func NewMongoPinger(client *mongo.Client) Pinger {
	return &MongoPinger{Client: client}
}

func (p MongoPinger) Ping(ctx context.Context) error {
	return p.Client.Ping(ctx, readpref.Primary())
}
