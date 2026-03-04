package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientFailover(t *testing.T) {
	var primaryHits int32
	var fallbackHits int32

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer primary.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fallbackHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallback.Close()

	client := NewHTTPClient(2*time.Second, ResilienceOptions{
		RetryAttempts:       0,
		RetryBackoff:        10 * time.Millisecond,
		FailureThreshold:    2,
		OpenTimeout:         200 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	resp, err := client.PostWithFailover(
		context.Background(),
		primary.URL+"/chat/completions",
		[]string{fallback.URL + "/chat/completions"},
		map[string]string{"hello": "world"},
		nil,
	)
	if err != nil {
		t.Fatalf("expected failover success, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from fallback, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&primaryHits) == 0 {
		t.Fatalf("expected primary to be attempted at least once")
	}
	if atomic.LoadInt32(&fallbackHits) == 0 {
		t.Fatalf("expected fallback to be attempted")
	}
}

func TestHTTPClientCircuitBreakerHalfOpenRecovery(t *testing.T) {
	var hitCount int32
	var returnSuccess atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		if !returnSuccess.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(2*time.Second, ResilienceOptions{
		RetryAttempts:       0,
		RetryBackoff:        10 * time.Millisecond,
		FailureThreshold:    1,
		OpenTimeout:         60 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	_, err := client.GetWithFailover(context.Background(), srv.URL+"/health", nil, nil)
	if err == nil {
		t.Fatalf("expected first call to fail")
	}
	if got := atomic.LoadInt32(&hitCount); got != 1 {
		t.Fatalf("expected first attempt to hit upstream once, got %d", got)
	}

	_, err = client.GetWithFailover(context.Background(), srv.URL+"/health", nil, nil)
	if err == nil {
		t.Fatalf("expected second call to fail fast due to open circuit")
	}
	if got := atomic.LoadInt32(&hitCount); got != 1 {
		t.Fatalf("expected open circuit to skip upstream, got %d hits", got)
	}

	time.Sleep(80 * time.Millisecond)
	returnSuccess.Store(true)

	resp, err := client.GetWithFailover(context.Background(), srv.URL+"/health", nil, nil)
	if err != nil {
		t.Fatalf("expected half-open recovery call to succeed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after recovery, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hitCount); got != 2 {
		t.Fatalf("expected upstream hit count to be 2 after recovery, got %d", got)
	}
}
