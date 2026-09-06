package obs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveChallengeCounts(t *testing.T) {
	before := testutil.ToFloat64(ChallengesTotal.WithLabelValues("present", "success"))
	ObserveChallenge("present", nil)
	ObserveChallenge("present", errors.New("x"))
	after := testutil.ToFloat64(ChallengesTotal.WithLabelValues("present", "success"))
	if after != before+1 {
		t.Fatalf("success counter = %v, want %v", after, before+1)
	}
}

func TestObserveAPIRequestOutcomeBuckets(t *testing.T) {
	start := time.Now().Add(-10 * time.Millisecond)
	ObserveAPIRequest("GET", 200, nil, start)
	ObserveAPIRequest("GET", 503, nil, start)
	ObserveAPIRequest("GET", 0, errors.New("dial"), start)
	if got := testutil.ToFloat64(APIRequestsTotal.WithLabelValues("GET", "503")); got < 1 {
		t.Fatalf("expected 503 counter incremented, got %v", got)
	}
}

func TestServeMetricsServesAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeMetrics(ctx, "127.0.0.1:0") }()
	// give the listener a moment, then shut down
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeMetrics returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeMetrics did not shut down")
	}
}

func TestServeMetricsDisabledWhenEmpty(t *testing.T) {
	if err := ServeMetrics(context.Background(), ""); err != nil {
		t.Fatalf("empty addr should be a no-op, got %v", err)
	}
}
