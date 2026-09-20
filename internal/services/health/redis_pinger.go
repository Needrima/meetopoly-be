package health

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// RedisPinger adapts *redis.Client to health.Pinger.
type RedisPinger struct {
	Client *goredis.Client
}

func NewRedisPinger(client *goredis.Client) Pinger {
	return &RedisPinger{Client: client}
}

func (p RedisPinger) Ping(ctx context.Context) error {
	return p.Client.Ping(ctx).Err()
}
