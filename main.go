// Command cert-manager-webhook-arvancloud runs the cert-manager ACME DNS-01
// solver webhook apiserver for ArvanCloud.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/obs"
	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/provider"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	_ = version

	// Diagnostic server (Prometheus /metrics + /healthz) on a separate
	// port. Override or disable with METRICS_BIND_ADDRESS ("" disables).
	metricsAddr := ":8081"
	if v, ok := os.LookupEnv("METRICS_BIND_ADDRESS"); ok {
		metricsAddr = v
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := obs.ServeMetrics(ctx, metricsAddr); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	cmd.RunWebhookServer(provider.GroupName, provider.Solver())
}
