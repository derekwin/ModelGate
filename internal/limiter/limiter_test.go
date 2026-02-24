package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(nil, 60)

	if rl.rpm != 60 {
		t.Errorf("RateLimiter.rpm = %d, want %d", rl.rpm, 60)
	}

	if rl.duration != time.Minute {
		t.Errorf("RateLimiter.duration = %v, want %v", rl.duration, time.Minute)
	}
}

func TestRateLimiter_IsAllowed(t *testing.T) {
	ctx := context.Background()
	
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	defer client.Close()

	err := client.FlushDB(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to flush Redis: %v", err)
	}

	rl := NewRateLimiter(client, 3)

	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{"first request", "limiter_test_user1", true},
		{"second request", "limiter_test_user1", true},
		{"third request", "limiter_test_user1", true},
		{"fourth request (should be blocked)", "limiter_test_user1", false},
		{" different user", "limiter_test_user2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := rl.IsAllowed(ctx, tt.key)
			if err != nil {
				t.Errorf("IsAllowed() error = %v", err)
			}
			if allowed != tt.expected {
				t.Errorf("IsAllowed() = %v, want %v", allowed, tt.expected)
			}
		})
	}
}

func TestRateLimiter_Expiry(t *testing.T) {
	ctx := context.Background()
	
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	defer client.Close()

	err := client.FlushDB(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to flush Redis: %v", err)
	}

	rl := NewRateLimiter(client, 2)

	for i := 0; i < 3; i++ {
		rl.IsAllowed(ctx, "limiter_test_expiry")
	}

	ttl, err := client.TTL(ctx, "limiter_test_expiry").Result()
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}

	if ttl <= 0 {
		t.Errorf("TTL should be positive, got %v", ttl)
	}

	if ttl > time.Minute {
		t.Errorf("TTL should be <= 1 minute, got %v", ttl)
	}
}
