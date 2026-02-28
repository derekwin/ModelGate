package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type RateLimiter struct {
	redis *redis.Client
	rpm   int
	burst int
}

func NewRateLimiter(redisAddr string, redisPassword string, redisDB int, rpm int, burst int) (*RateLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RateLimiter{
		redis: client,
		rpm:   rpm,
		burst: burst,
	}, nil
}

func (r *RateLimiter) Allow(key string) (bool, error) {
	ctx := context.Background()

	now := time.Now()
	rateKey := fmt.Sprintf("rate:%s", key)

	pipe := r.redis.Pipeline()

	pipe.ZRemRangeByScore(ctx, rateKey, "0", fmt.Sprintf("%d", now.Add(-time.Minute).UnixMilli()))

	pipe.ZAdd(ctx, rateKey, &redis.Z{
		Score:  float64(now.UnixMilli()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	pipe.Expire(ctx, rateKey, time.Minute)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("redis error: %w", err)
	}

	currentCount := r.redis.ZCard(ctx, rateKey).Val()

	return currentCount <= int64(r.rpm), nil
}

func (r *RateLimiter) GetRemaining(key string) (int, error) {
	ctx := context.Background()
	rateKey := fmt.Sprintf("rate:%s", key)

	count, err := r.redis.ZCard(ctx, rateKey).Result()
	if err != nil {
		return 0, err
	}

	remaining := r.rpm - int(count)
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

func (r *RateLimiter) Close() error {
	return r.redis.Close()
}
