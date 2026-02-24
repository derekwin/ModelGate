package usage

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func TestNewUsageTracker(t *testing.T) {
	tracker := NewUsageTracker(nil, time.Hour, 1000)

	if tracker.period != time.Hour {
		t.Errorf("UsageTracker.period = %v, want %v", tracker.period, time.Hour)
	}

	if tracker.limit != 1000 {
		t.Errorf("UsageTracker.limit = %d, want %d", tracker.limit, 1000)
	}
}

func TestUsageTracker_Track(t *testing.T) {
	ctx := context.Background()

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	defer client.Close()

	err := client.FlushDB(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to flush Redis: %v", err)
	}

	tracker := NewUsageTracker(client, time.Hour, 100)

	allowed, err := tracker.Track(ctx, "user_test", 10)
	if err != nil {
		t.Errorf("Track() error = %v", err)
	}
	if !allowed {
		t.Error("Track() = false, want true (first track should succeed)")
	}

	allowed, err = tracker.Track(ctx, "user_test", 20)
	if err != nil {
		t.Errorf("Track() error = %v", err)
	}
	if !allowed {
		t.Error("Track() = false, want true (second track should succeed)")
	}

	allowed, err = tracker.Track(ctx, "user_test", 30)
	if err != nil {
		t.Errorf("Track() error = %v", err)
	}
	if !allowed {
		t.Error("Track() = false, want true (third track should succeed)")
	}

	current, err := client.Get(ctx, "usage:user_test:current").Int64()
	if err != nil {
		t.Fatalf("Failed to get current: %v", err)
	}
	if current != 60 {
		t.Errorf("Expected current = 60, got %d", current)
	}
}

func TestUsageTracker_TrackWithLimit(t *testing.T) {
	ctx := context.Background()

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	defer client.Close()

	err := client.FlushDB(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to flush Redis: %v", err)
	}

	tracker := NewUsageTracker(client, time.Hour, 50)

	allowed, err := tracker.Track(ctx, "user_limit", 20)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if !allowed {
		t.Error("First track should be allowed")
	}

	allowed, err = tracker.Track(ctx, "user_limit", 20)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if !allowed {
		t.Error("Second track should be allowed")
	}

	allowed, err = tracker.Track(ctx, "user_limit", 20)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if allowed {
		t.Error("Third track should be blocked (exceeds limit)")
	}
}
