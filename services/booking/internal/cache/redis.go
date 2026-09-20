package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func NewRedisCache(ctx context.Context, redisURL string) *Cache {

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(opts)

	// Quick health check
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic("Could not connect to Redis: " + err.Error())
	}

	return &Cache{client: rdb}
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
