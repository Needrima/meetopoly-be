package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const keyPrefix = "session:"

// ErrNotFound is returned when the token is missing or expired.
var ErrNotFound = errors.New("session not found")

// RedisRepository implements Repository with Redis.
type RedisRepository struct {
	client *goredis.Client
}

// NewRedisRepository wraps a Redis client.
func NewRedisRepository(client *goredis.Client) *RedisRepository {
	return &RedisRepository{client: client}
}

func (r *RedisRepository) Create(ctx context.Context, token string, data TokenData, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("session marshal: %w", err)
	}
	// ttl == 0 → no expiry (login sessions until logout).
	if err := r.client.Set(ctx, keyPrefix+token, payload, ttl).Err(); err != nil {
		return fmt.Errorf("session set: %w", err)
	}
	return nil
}

func (r *RedisRepository) Get(ctx context.Context, token string) (*TokenData, error) {
	raw, err := r.client.Get(ctx, keyPrefix+token).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("session get: %w", err)
	}
	var data TokenData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("session unmarshal: %w", err)
	}
	return &data, nil
}

func (r *RedisRepository) Delete(ctx context.Context, token string) error {
	if err := r.client.Del(ctx, keyPrefix+token).Err(); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}
