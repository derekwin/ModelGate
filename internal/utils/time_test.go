package utils

import (
	"testing"
	"time"
)

func TestCurrentTimestamp(t *testing.T) {
	expected := time.Now().Unix()
	timestamp := CurrentTimestamp()

	if timestamp < expected {
		t.Errorf("CurrentTimestamp() = %d, expected >= %d", timestamp, expected)
	}

	diff := timestamp - expected
	if diff > 2 {
		t.Errorf("CurrentTimestamp() returned old timestamp, diff = %d", diff)
	}
}

func TestCurrentTimestampIncreasing(t *testing.T) {
	expected := time.Now().Unix()
	time.Sleep(1 * time.Second)
	timestamp := CurrentTimestamp()

	if timestamp <= expected {
		t.Errorf("CurrentTimestamp() not increasing: %d <= %d", timestamp, expected)
	}
}
