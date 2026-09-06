// Package provider implements the cert-manager ACME DNS-01 webhook Solver
// interface for ArvanCloud.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook"
	whapi "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/client"
	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/obs"
)

// GroupName is the API group the webhook registers under. It must match the
// `groupName` field of the ACME Issuer's `dns01.webhook` config.
const GroupName = "acme.arvancloud.ir"

const (
	defaultTTL = 120
	solverName = "arvancloud"
)

// arvanAPI is the subset of the ArvanCloud client the solver depends on,
// extracted as an interface so tests can substitute a fake.
type arvanAPI interface {
	CreateTXTRecord(ctx context.Context, domain, name, value string, ttl int) (string, error)
	FindTXTRecords(ctx context.Context, domain, name string) ([]client.Record, error)
	DeleteRecord(ctx context.Context, domain, id string) error
}

// arvanDNSProviderConfig is the per-Issuer configuration block, supplied as
// unstructured JSON in the ACME Issuer manifest.
type arvanDNSProviderConfig struct {
	// APIKeySecretRef references the Secret key holding the ArvanCloud
	// Machine User API key. Required.
	APIKeySecretRef cmmeta.SecretKeySelector `json:"apiKeySecretRef"`
	// APIURL overrides the ArvanCloud API base URL. Optional.
	APIURL string `json:"apiUrl,omitempty"`
	// TTL for the challenge TXT record, in seconds. Defaults to 120.
	TTL int `json:"ttl,omitempty"`
	// PropagationCheck, when true, makes Present block until the record is
	// visible on public resolvers (1.1.1.1, 8.8.8.8) before returning.
	// Defaults to false: cert-manager already runs its own DNS self-check
	// (--dns01-recursive-nameservers) before asking the ACME server to
	// validate, so this is redundant and only adds latency to the
	// synchronous webhook call. Enable it only if that self-check is
	// disabled or misconfigured.
	PropagationCheck *bool `json:"propagationCheck,omitempty"`
	// PropagationTimeoutSeconds bounds the propagation wait. Defaults to 45.
	// Keep it below the webhook apiserver request timeout (60s) unless you
	// have raised --request-timeout for the webhook.
	PropagationTimeoutSeconds int `json:"propagationTimeoutSeconds,omitempty"`
}

func (c arvanDNSProviderConfig) propagationCheckEnabled() bool {
	return c.PropagationCheck != nil && *c.PropagationCheck
}

// Solver returns a cert-manager webhook.Solver backed by ArvanCloud DNS.
func Solver() webhook.Solver {
	return &arvanDNSProviderSolver{
		newAPIClient: func(apiKey, baseURL string) (arvanAPI, error) {
			return client.New(apiKey, client.WithBaseURL(baseURL))
		},
	}
}

type arvanDNSProviderSolver struct {
	kube         kubernetes.Interface
	newAPIClient func(apiKey, baseURL string) (arvanAPI, error)
}

func (s *arvanDNSProviderSolver) Name() string { return solverName }

// Initialize wires up the in-cluster Kubernetes client used to read the
// API-key Secret.
func (s *arvanDNSProviderSolver) Initialize(kubeCfg *rest.Config, _ <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return err
	}
	s.kube = cl
	return nil
}

func (s *arvanDNSProviderSolver) Present(ch *whapi.ChallengeRequest) (err error) {
	defer func() { obs.ObserveChallenge("present", err) }()
	ctx := context.Background()

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	api, err := s.apiClient(ctx, cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	domain, name := zoneAndName(ch)
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}

	if _, err = api.CreateTXTRecord(ctx, domain, name, ch.Key, ttl); err != nil {
		return fmt.Errorf("presenting challenge for %q: %w", ch.ResolvedFQDN, err)
	}

	if cfg.propagationCheckEnabled() {
		if err = waitForPropagation(ctx, ch.ResolvedFQDN, ch.Key, cfg.PropagationTimeoutSeconds); err != nil {
			return err
		}
	}
	return nil
}

// CleanUp removes the challenge record. It is idempotent: a missing record is
// treated as success.
func (s *arvanDNSProviderSolver) CleanUp(ch *whapi.ChallengeRequest) (err error) {
	defer func() { obs.ObserveChallenge("cleanup", err) }()
	ctx := context.Background()

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	api, err := s.apiClient(ctx, cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	domain, name := zoneAndName(ch)

	records, err := api.FindTXTRecords(ctx, domain, name)
	if err != nil {
		return fmt.Errorf("cleaning up challenge for %q: %w", ch.ResolvedFQDN, err)
	}
	for _, r := range records {
		if r.Value.Text != ch.Key {
			continue
		}
		if err := api.DeleteRecord(ctx, domain, r.ID); err != nil && !errors.Is(err, client.ErrRecordNotFound) {
			return fmt.Errorf("deleting record %s for %q: %w", r.ID, ch.ResolvedFQDN, err)
		}
	}
	return nil
}

func (s *arvanDNSProviderSolver) apiClient(ctx context.Context, cfg arvanDNSProviderConfig, namespace string) (arvanAPI, error) {
	key, err := s.resolveAPIKey(ctx, cfg.APIKeySecretRef, namespace)
	if err != nil {
		return nil, err
	}
	return s.newAPIClient(key, cfg.APIURL)
}

func (s *arvanDNSProviderSolver) resolveAPIKey(ctx context.Context, ref cmmeta.SecretKeySelector, namespace string) (string, error) {
	if s.kube == nil {
		return "", errors.New("solver not initialized: kubernetes client is nil")
	}
	if ref.Name == "" || ref.Key == "" {
		return "", errors.New("apiKeySecretRef must set both name and key")
	}
	secret, err := s.kube.CoreV1().Secrets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("loading secret %s/%s: %w", namespace, ref.Name, err)
	}
	raw, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name)
	}
	if v := strings.TrimSpace(string(raw)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("key %q in secret %s/%s is empty", ref.Key, namespace, ref.Name)
}

// zoneAndName splits a ChallengeRequest into the ArvanCloud domain (zone apex,
// no trailing dot) and the zone-relative record name. It handles multi-level
// subdomains such as `_acme-challenge.apps.cluster.example.com.`.
func zoneAndName(ch *whapi.ChallengeRequest) (domain, name string) {
	domain = strings.TrimSuffix(ch.ResolvedZone, ".")
	fqdn := strings.TrimSuffix(ch.ResolvedFQDN, ".")
	name = strings.TrimSuffix(fqdn, "."+domain)
	if name == fqdn || name == "" {
		name = "@"
	}
	return domain, name
}

// loadConfig unmarshals the raw Issuer webhook config.
func loadConfig(raw *extapi.JSON) (arvanDNSProviderConfig, error) {
	cfg := arvanDNSProviderConfig{}
	if raw == nil {
		return cfg, errors.New("no webhook config provided: apiKeySecretRef is required")
	}
	if err := json.Unmarshal(raw.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("decoding webhook config: %w", err)
	}
	return cfg, nil
}

// compile-time guard
var _ webhook.Solver = (*arvanDNSProviderSolver)(nil)
