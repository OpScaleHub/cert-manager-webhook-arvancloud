package provider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/obs"
)

// publicResolvers are queried directly (bypassing the pod's configured
// resolver) to confirm a challenge TXT record is publicly visible before
// Present hands control back to cert-manager.
var publicResolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}

const (
	// defaultPropagationTimeout leaves headroom under the webhook
	// apiserver's default 60s --request-timeout.
	defaultPropagationTimeout = 45 * time.Second
	propagationInterval       = 5 * time.Second
	// perLookupTimeout bounds a single resolver query so a blocked or
	// slow-pathed egress to port 53 fails fast instead of stacking dial
	// timeouts inside a single check cycle.
	perLookupTimeout = 4 * time.Second
)

// waitForPropagation polls the public resolvers until every one of them
// returns the expected TXT value for fqdn, or the timeout expires.
// timeoutSeconds <= 0 selects defaultPropagationTimeout.
func waitForPropagation(ctx context.Context, fqdn, expected string, timeoutSeconds int) (err error) {
	start := time.Now()
	defer func() { obs.ObservePropagationWait(start, err) }()

	timeout := defaultPropagationTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Keep the name fully qualified (trailing dot). Without it, Go's
	// resolver applies /etc/resolv.conf's search list and ndots — in a
	// Kubernetes pod that fans every check into several bogus
	// *.svc.cluster.local queries against the public resolver, which can
	// stall for many seconds.
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	ticker := time.NewTicker(propagationInterval)
	defer ticker.Stop()

	for {
		if allResolversSee(ctx, fqdn, expected) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s waiting for TXT %q to reach public resolvers", timeout, fqdn)
		case <-ticker.C:
		}
	}
}

func allResolversSee(ctx context.Context, fqdn, expected string) bool {
	for _, addr := range publicResolvers {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: perLookupTimeout}
				return d.DialContext(ctx, network, addr)
			},
		}
		lookupCtx, cancel := context.WithTimeout(ctx, perLookupTimeout)
		values, err := r.LookupTXT(lookupCtx, fqdn)
		cancel()
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
