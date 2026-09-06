//go:build conformance

// Package provider conformance test.
//
// Runs cert-manager's official external-webhook conformance suite
// (test/acme) against the *real* ArvanCloud API — the standard proof every
// community DNS-01 solver ships beyond unit tests against mocks. It spins up
// a local envtest control plane, applies the API-key Secret + webhook
// config, and drives Present/CleanUp with a synthetic challenge (no ACME
// order, no Let's Encrypt rate limits).
//
// Excluded from the normal `go test ./...` run by the `conformance` build
// tag. Requires:
//
//   - envtest binaries — either KUBEBUILDER_ASSETS, or cert-manager's own
//     TEST_ASSET_ETCD / TEST_ASSET_KUBE_APISERVER / TEST_ASSET_KUBECTL:
//
//     ASSETS="$(setup-envtest use 1.31.0 -p path)"
//     export TEST_ASSET_ETCD="$ASSETS/etcd"
//     export TEST_ASSET_KUBE_APISERVER="$ASSETS/kube-apiserver"
//     export TEST_ASSET_KUBECTL="$ASSETS/kubectl"
//
//   - ARVANCLOUD_API_KEY — a Machine User key with the DNS-records
//     permission *explicitly granted* for TEST_ZONE_NAME. A Machine User
//     without that scope gets "HTTP 403: Your access to this section is
//     restricted." from every call, not a clear permission-denied error.
//
//   - TEST_ZONE_NAME — a real ArvanCloud-managed zone, trailing dot,
//     e.g. "opscale.ir." (normaliseName strips it either way, but the
//     fixture expects the fully-qualified form).
//
// Run: go test -tags conformance -run TestConformance ./internal/provider/ -v -timeout 15m
package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

func TestConformance(t *testing.T) {
	apiKey := os.Getenv("ARVANCLOUD_API_KEY")
	zone := os.Getenv("TEST_ZONE_NAME")
	if apiKey == "" || zone == "" {
		t.Skip("set ARVANCLOUD_API_KEY and TEST_ZONE_NAME to run the conformance suite")
	}

	manifestDir := t.TempDir()
	secret := fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: arvancloud-credentials\ntype: Opaque\nstringData:\n  api-key: %q\n", apiKey)
	if err := os.WriteFile(filepath.Join(manifestDir, "secret.yaml"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(map[string]any{
		"apiKeySecretRef":  map[string]any{"name": "arvancloud-credentials", "key": "api-key"},
		"propagationCheck": false,
	})

	fixture := acmetest.NewFixture(Solver(),
		acmetest.SetResolvedZone(zone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath(manifestDir),
		acmetest.SetConfig(json.RawMessage(cfg)),
		// Verify the challenge record against the zone's authoritative
		// ArvanCloud nameservers directly — they answer immediately,
		// whereas a public recursive resolver would make the test wait
		// on cache TTLs.
		acmetest.SetUseAuthoritative(true),
	)
	fixture.RunConformance(t)
}
