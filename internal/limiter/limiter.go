package limiter

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type RateLimiter struct {
	redis    *redis.Client
	rpm      int
	duration time.Duration
}

func NewRateLimiter(redis *redis.Client, rpm int) *RateLimiter {
	return &RateLimiter{
		redis:    redis,
		rpm:      rpm,
		duration: time.Minute,
	}
}

func (rl *RateLimiter) IsAllowed(ctx context.Context, key string) (bool, error) {
	count, err := rl.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		rl.redis.Expire(ctx, key, rl.duration)
	}

	if count > int64(rl.rpm) {
		return false, nil
	}

	return true, nil
}
