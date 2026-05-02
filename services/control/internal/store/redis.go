package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type noopRedis struct{}

func NewNoopRedis() Redis {
	return noopRedis{}
}

func (noopRedis) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (noopRedis) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	_ = ctx
	_ = key
	_ = v
	_ = ttl
	return nil
}

func (noopRedis) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	_ = ctx
	_ = key
	_ = dest
	return false, nil
}

func (noopRedis) AcquireLock(ctx context.Context, key string, ttl time.Duration) (RedisLock, bool, error) {
	_ = ctx
	_ = key
	_ = ttl
	return nil, false, nil
}

type redisClient struct {
	c *redis.Client
}

func NewRedis(ctx context.Context, url string) (Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &redisClient{c: c}, nil
}

func (r *redisClient) Ping(ctx context.Context) error {
	return r.c.Ping(ctx).Err()
}

func (r *redisClient) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return r.c.Set(ctx, key, b, ttl).Err()
}

func (r *redisClient) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	b, err := r.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return false, err
	}
	return true, nil
}

type redisLock struct {
	c     *redis.Client
	key   string
	token string
}

var redisUnlockScript = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)

func (r *redisClient) AcquireLock(ctx context.Context, key string, ttl time.Duration) (RedisLock, bool, error) {
	token := uuid.NewString()
	ok, err := r.c.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return &redisLock{c: r.c, key: key, token: token}, true, nil
}

func (l *redisLock) Release(ctx context.Context) error {
	_, err := redisUnlockScript.Run(ctx, l.c, []string{l.key}, l.token).Result()
	return err
}

