package usage

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type UsageTracker struct {
	redis   *redis.Client
	period  time.Duration
	limit   int64
}

func NewUsageTracker(redis *redis.Client, period time.Duration, limit int64) *UsageTracker {
	return &UsageTracker{
		redis:   redis,
		period:  period,
		limit:   limit,
	}
}

func (ut *UsageTracker) Track(ctx context.Context, userID string, tokens int64) (bool, error) {
	currentKey := "usage:" + userID + ":current"

	pipe := ut.redis.Pipeline()

	pipe.IncrBy(ctx, currentKey, tokens)
	pipe.Expire(ctx, currentKey, ut.period)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	current, err := ut.redis.Get(ctx, currentKey).Int64()
	if err != nil {
		return false, err
	}

	if ut.limit > 0 && current > ut.limit {
		return false, nil
	}

	return true, nil
}

func (ut *UsageTracker) Reset(ctx context.Context, userID string) error {
	currentKey := "usage:" + userID + ":current"
	return ut.redis.Del(ctx, currentKey).Err()
}
