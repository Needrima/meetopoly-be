package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	keyPrefix          = "session:"
	userSessionsPrefix = "user_sessions:"
)

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
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, keyPrefix+token, payload, ttl)
	// Index login sessions so we can revoke all for a user (password reset).
	if data.Kind == KindSession && data.UserID != "" {
		pipe.SAdd(ctx, userSessionsPrefix+data.UserID, token)
	}
	if _, err := pipe.Exec(ctx); err != nil {
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
	data, err := r.Get(ctx, token)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	pipe := r.client.TxPipeline()
	pipe.Del(ctx, keyPrefix+token)
	if data != nil && data.Kind == KindSession && data.UserID != "" {
		pipe.SRem(ctx, userSessionsPrefix+data.UserID, token)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}

func (r *RedisRepository) DeleteAllForUser(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	setKey := userSessionsPrefix + userID
	tokens, err := r.client.SMembers(ctx, setKey).Result()
	if err != nil {
		return fmt.Errorf("session list user: %w", err)
	}
	if len(tokens) == 0 {
		_ = r.client.Del(ctx, setKey).Err()
		return nil
	}
	pipe := r.client.TxPipeline()
	for _, t := range tokens {
		pipe.Del(ctx, keyPrefix+t)
	}
	pipe.Del(ctx, setKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session delete all: %w", err)
	}
	return nil
}
