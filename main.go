// Command cert-manager-webhook-arvancloud runs the cert-manager ACME DNS-01
// solver webhook apiserver for ArvanCloud.
package main

import (
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/provider"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	_ = version
	cmd.RunWebhookServer(provider.GroupName, provider.Solver())
}
