// Package obs holds the webhook's observability surface: Prometheus metrics
// and the diagnostic HTTP server that exposes them.
package obs

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// APIRequestDuration tracks ArvanCloud API call latency by HTTP method
	// and outcome ("2xx", "4xx", "5xx", "error").
	APIRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "arvancloud_webhook",
		Subsystem: "api",
		Name:      "request_duration_seconds",
		Help:      "Duration of ArvanCloud DNS API requests.",
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 15},
	}, []string{"method", "outcome"})

	// APIRequestsTotal counts ArvanCloud API calls by method and HTTP
	// status code (or "error" for transport failures).
	APIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "arvancloud_webhook",
		Subsystem: "api",
		Name:      "requests_total",
		Help:      "Total ArvanCloud DNS API requests by status.",
	}, []string{"method", "code"})

	// ChallengesTotal counts solver operations by action ("present" /
	// "cleanup") and result ("success" / "error").
	ChallengesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "arvancloud_webhook",
		Subsystem: "solver",
		Name:      "challenges_total",
		Help:      "Total DNS-01 challenge operations handled by the webhook.",
	}, []string{"action", "result"})

	// PropagationWaitDuration tracks how long Present blocked on the
	// optional public-resolver propagation check, by outcome ("ok" /
	// "timeout").
	PropagationWaitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "arvancloud_webhook",
		Subsystem: "solver",
		Name:      "propagation_wait_seconds",
		Help:      "Time Present spent waiting for the challenge record to reach public resolvers.",
		Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 45},
	}, []string{"outcome"})
)

// ObservePropagationWait records one propagation-check wait.
func ObservePropagationWait(start time.Time, err error) {
	outcome := "ok"
	if err != nil {
		outcome = "timeout"
	}
	PropagationWaitDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
}

// ObserveAPIRequest records a single ArvanCloud API call.
func ObserveAPIRequest(method string, statusCode int, err error, start time.Time) {
	outcome := "error"
	code := "error"
	switch {
	case err != nil:
	case statusCode >= 500:
		outcome, code = "5xx", strconv.Itoa(statusCode)
	case statusCode >= 400:
		outcome, code = "4xx", strconv.Itoa(statusCode)
	case statusCode >= 200:
		outcome, code = "2xx", strconv.Itoa(statusCode)
	default:
		outcome, code = "1xx", strconv.Itoa(statusCode)
	}
	APIRequestDuration.WithLabelValues(method, outcome).Observe(time.Since(start).Seconds())
	APIRequestsTotal.WithLabelValues(method, code).Inc()
}

// ObserveChallenge records the result of a Present or CleanUp call.
func ObserveChallenge(action string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	ChallengesTotal.WithLabelValues(action, result).Inc()
}

// ServeMetrics starts a diagnostic HTTP server exposing /metrics and
// /healthz on addr. It blocks until ctx is cancelled, then shuts down
// gracefully. An empty addr disables the server.
func ServeMetrics(ctx context.Context, addr string) error {
	if addr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
