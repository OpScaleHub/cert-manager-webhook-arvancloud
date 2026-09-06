package main

import (
	"encoding/base64"
	"errors"
	"os"
	"testing"
	"text/template"

	acmetest "github.com/cert-manager/cert-manager/test/acme"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/provider"
)

var (
	testZoneName = os.Getenv("TEST_ZONE_NAME")
	manifestPath = "testdata/arvancloud"
)

// createSecretFile writes the API-key Secret manifest the fixture applies
// into the test cluster before each conformance sub-test. Matches
// solver.go's resolveAPIKey expectations exactly: Secret name
// "arvancloud-credentials", key "api-key".
func createSecretFile() error {
	apiKey := os.Getenv("ARVANCLOUD_API_KEY")
	if apiKey == "" {
		return errors.New("ARVANCLOUD_API_KEY must be set")
	}
	apiKeyBase64 := base64.StdEncoding.EncodeToString([]byte(apiKey))

	secretTmpl := `---
apiVersion: v1
kind: Secret
metadata:
  name: arvancloud-credentials
type: Opaque
data:
  api-key: {{.}}
`
	secretFile, err := os.Create(manifestPath + "/api-key.yaml")
	if err != nil {
		return err
	}
	defer secretFile.Close()

	tmpl, err := template.New("api-key.yaml").Parse(secretTmpl)
	if err != nil {
		return err
	}
	return tmpl.Execute(secretFile, apiKeyBase64)
}

// createConfig writes the per-Issuer webhook config snippet, matching
// arvanDNSProviderConfig's JSON shape in solver.go.
func createConfig() error {
	config := []byte(`{
	"apiKeySecretRef": {
		"name": "arvancloud-credentials",
		"key": "api-key"
	}
}
`)
	return os.WriteFile(manifestPath+"/config.json", config, 0644)
}

func runTestSuite(t *testing.T, zone string) {
	if len(zone) == 0 {
		t.Fatal("TEST_ZONE_NAME must be set to a real ArvanCloud-managed zone")
	}

	if err := createSecretFile(); err != nil {
		t.Fatal(err)
	}
	if err := createConfig(); err != nil {
		t.Fatal(err)
	}

	fixture := acmetest.NewFixture(provider.Solver(),
		acmetest.SetResolvedZone(zone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath(manifestPath),
		// ArvanCloud's own authoritative nameservers answer the TXT
		// challenge query well before it'd ever reach a public
		// recursive resolver -- querying the authoritative server
		// directly avoids waiting on public-resolver cache TTLs
		// during the test.
		acmetest.SetUseAuthoritative(true),
	)

	fixture.RunConformance(t)
}

// TestRunsSuite is the standard conformance entry point. Run with:
//
//	TEST_ZONE_NAME=opscale.ir. ARVANCLOUD_API_KEY=... go test -v .
//
// Trailing dot on TEST_ZONE_NAME matches cert-manager's own convention
// (a fully-qualified zone name) -- the solver's zoneAndName/normaliseName
// already strip it either way, but the fixture itself expects it.
func TestRunsSuite(t *testing.T) {
	runTestSuite(t, testZoneName)
}
