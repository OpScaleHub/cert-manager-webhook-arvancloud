package provider

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	whapi "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/client"
)

func secretRef(name, key string) cmmeta.SecretKeySelector {
	return cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: name},
		Key:                  key,
	}
}

func recordValue(text string) client.RecordValue { return client.RecordValue{Text: text} }

type fakeAPI struct {
	created []string
	records []client.Record
	deleted []string
}

func (f *fakeAPI) CreateTXTRecord(_ context.Context, domain, name, value string, ttl int) (string, error) {
	f.created = append(f.created, domain+"|"+name+"|"+value)
	id := "rec-new"
	f.records = append(f.records, client.Record{ID: id, Type: "txt", Name: name})
	return id, nil
}

func (f *fakeAPI) FindTXTRecords(_ context.Context, _, _ string) ([]client.Record, error) {
	return f.records, nil
}

func (f *fakeAPI) DeleteRecord(_ context.Context, _, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func newSolver(t *testing.T, api arvanAPI, secrets ...*corev1.Secret) *arvanDNSProviderSolver {
	t.Helper()
	cl := fake.NewSimpleClientset()
	for _, sec := range secrets {
		if _, err := cl.CoreV1().Secrets(sec.Namespace).Create(context.Background(), sec, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed secret: %v", err)
		}
	}
	return &arvanDNSProviderSolver{
		kube:         cl,
		newAPIClient: func(_, _ string) (arvanAPI, error) { return api, nil },
	}
}

func testSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "arvan-credentials", Namespace: "cert-manager"},
		Data:       map[string][]byte{"api-key": []byte("machine-user-key")},
	}
}

func boolPtr(b bool) *bool { return &b }

func challenge(fqdn, zone, key string) *whapi.ChallengeRequest {
	cfg, _ := json.Marshal(arvanDNSProviderConfig{
		APIKeySecretRef:  secretRef("arvan-credentials", "api-key"),
		PropagationCheck: boolPtr(false),
	})
	return &whapi.ChallengeRequest{
		ResolvedFQDN:      fqdn,
		ResolvedZone:      zone,
		Key:               key,
		ResourceNamespace: "cert-manager",
		Config:            &extapi.JSON{Raw: cfg},
	}
}

func TestPresentCreatesRecord(t *testing.T) {
	api := &fakeAPI{}
	s := newSolver(t, api, testSecret())

	err := s.Present(challenge("_acme-challenge.www.example.com.", "example.com.", "tok"))
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if len(api.created) != 1 || api.created[0] != "example.com|_acme-challenge.www|tok" {
		t.Fatalf("created = %v", api.created)
	}
}

func TestPresentMultiLevelSubdomain(t *testing.T) {
	api := &fakeAPI{}
	s := newSolver(t, api, testSecret())

	if err := s.Present(challenge("_acme-challenge.apps.cluster.example.com.", "example.com.", "tok")); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if api.created[0] != "example.com|_acme-challenge.apps.cluster|tok" {
		t.Fatalf("created = %v", api.created)
	}
}

func TestCleanUpDeletesMatchingRecord(t *testing.T) {
	api := &fakeAPI{records: []client.Record{
		{ID: "keep", Type: "txt", Name: "_acme-challenge", Value: recordValue("other")},
		{ID: "drop", Type: "txt", Name: "_acme-challenge", Value: recordValue("tok")},
	}}
	s := newSolver(t, api, testSecret())

	if err := s.CleanUp(challenge("_acme-challenge.example.com.", "example.com.", "tok")); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if len(api.deleted) != 1 || api.deleted[0] != "drop" {
		t.Fatalf("deleted = %v", api.deleted)
	}
}

func TestCleanUpIdempotentWhenNoRecords(t *testing.T) {
	api := &fakeAPI{}
	s := newSolver(t, api, testSecret())
	if err := s.CleanUp(challenge("_acme-challenge.example.com.", "example.com.", "tok")); err != nil {
		t.Fatalf("CleanUp should be nil when nothing to delete, got %v", err)
	}
}

func TestPresentMissingSecret(t *testing.T) {
	api := &fakeAPI{}
	s := newSolver(t, api) // no secret seeded
	if err := s.Present(challenge("_acme-challenge.example.com.", "example.com.", "tok")); err == nil {
		t.Fatal("expected error when secret is missing")
	}
}

func TestLoadConfigRejectsNil(t *testing.T) {
	if _, err := loadConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestPropagationCheckDefaultsOn(t *testing.T) {
	if !(arvanDNSProviderConfig{}).propagationCheckEnabled() {
		t.Fatal("propagation check should default to enabled")
	}
	if (arvanDNSProviderConfig{PropagationCheck: boolPtr(false)}).propagationCheckEnabled() {
		t.Fatal("explicit false should disable propagation check")
	}
}

func TestName(t *testing.T) {
	if got := (&arvanDNSProviderSolver{}).Name(); got != "arvancloud" {
		t.Fatalf("Name() = %q", got)
	}
}
