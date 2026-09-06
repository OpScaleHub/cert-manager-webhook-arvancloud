package provider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// publicResolvers are queried directly (bypassing the pod's configured
// resolver) to confirm a challenge TXT record is globally visible before
// Present hands control back to cert-manager.
var publicResolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}

const (
	// defaultPropagationTimeout leaves headroom under the webhook
	// apiserver's default 60s --request-timeout.
	defaultPropagationTimeout = 45 * time.Second
	propagationInterval       = 5 * time.Second
)

// waitForPropagation polls the public resolvers until every one of them
// returns the expected TXT value for fqdn, or the timeout expires.
// timeoutSeconds <= 0 selects defaultPropagationTimeout.
func waitForPropagation(ctx context.Context, fqdn, expected string, timeoutSeconds int) error {
	timeout := defaultPropagationTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fqdn = strings.TrimSuffix(fqdn, ".")
	ticker := time.NewTicker(propagationInterval)
	defer ticker.Stop()

	for {
		if allResolversSee(ctx, fqdn, expected) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for TXT %q to propagate to public resolvers", fqdn)
		case <-ticker.C:
		}
	}
}

func allResolversSee(ctx context.Context, fqdn, expected string) bool {
	for _, addr := range publicResolvers {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, addr)
			},
		}
		values, err := r.LookupTXT(ctx, fqdn)
		if err != nil || !contains(values, expected) {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
