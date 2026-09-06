//go:build conformance

// Package provider conformance test.
//
// This runs cert-manager's official external-webhook conformance suite
// (test/acme) against the *real* ArvanCloud API. It is excluded from the
// normal `go test ./...` run by the `conformance` build tag because it needs:
//
//   - envtest binaries (etcd, kube-apiserver, kubectl) on PATH or via
//     KUBEBUILDER_ASSETS — e.g. `setup-envtest use -p path`
//   - ARVANCLOUD_API_KEY  — a Machine User key scoped to DNS for TEST_ZONE_NAME
//   - TEST_ZONE_NAME      — a real zone you control, trailing dot, e.g. "example.ir."
//   - TEST_DNS_SERVER     — optional resolver, default "1.1.1.1:53"
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
	dnsServer := os.Getenv("TEST_DNS_SERVER")
	if dnsServer == "" {
		dnsServer = "1.1.1.1:53"
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
		acmetest.SetManifestPath(manifestDir),
		acmetest.SetConfig(json.RawMessage(cfg)),
		acmetest.SetDNSServer(dnsServer),
		acmetest.SetUseAuthoritative(false),
	)
	fixture.RunConformance(t)
}
