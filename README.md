# cert-manager-webhook-arvancloud

A production-grade [cert-manager](https://cert-manager.io) ACME **DNS-01**
webhook solver for [ArvanCloud](https://www.arvancloud.ir/) DNS, using the
ArvanCloud CDN/DNS API v4 and Machine User API keys.

It implements the official `webhook.Solver` interface
(`github.com/cert-manager/cert-manager/pkg/acme/webhook`) and runs as a small
extension-apiserver alongside cert-manager.

ArvanCloud API reference: <https://www.arvancloud.ir/docs/api/cdn/4.0> ·
[API usage / getting a key](https://docs.arvancloud.ir/en/developer-tools/api/api-usage) ·
[DNS records](https://docs.arvancloud.ir/en/cdn/dns-records/adding-records).
The client (`internal/client`) matches the endpoints and TXT payload shape
used by `go-acme/lego`'s ArvanCloud provider.

## Features

- **API v4** against `https://napi.arvancloud.ir`, `Authorization: Apikey <key>`.
- **Idempotent** `Present` (reuses an existing identical TXT record) and
  `CleanUp` (a missing record is success, not an error loop).
- **Hard 15s timeout** and bounded retries on every API call, so a slow
  ArvanCloud response never stalls a cert-manager queue worker.
- **Multi-level subdomain** handling (`_acme-challenge.apps.cluster.example.com`).
- **Public-resolver propagation check** (1.1.1.1 / 8.8.8.8) before `Present`
  returns — on by default, opt-out.
- **Pooled HTTP transport** shared process-wide (tuned keep-alives / idle conns).
- **Prometheus metrics** (`/metrics` on `:8081`): API latency histograms, error
  rates, challenge success/failure counters; optional `ServiceMonitor`.
- Credentials read **only** from a namespaced Kubernetes `Secret`; never logged.
- Restricted pod security context: non-root `10001`, read-only root FS, all
  capabilities dropped, `RuntimeDefault` seccomp.
- Multi-arch image (`linux/amd64`, `linux/arm64`), distroless base, **cosign
  keyless signature** + SBOM + provenance on every release.

## Install (Helm)

```sh
helm repo add opscalehub https://opscalehub.github.io/cert-manager-webhook-arvancloud
helm repo update
helm install arvancloud-webhook opscalehub/arvancloud-webhook \
  --namespace cert-manager --version <VERSION>
```

The chart is also published as an OCI artifact:

```sh
helm install arvancloud-webhook \
  oci://ghcr.io/opscalehub/charts/arvancloud-webhook \
  --namespace cert-manager --version <VERSION>
```

Or straight from this repo:

```sh
helm install arvancloud-webhook ./charts/arvancloud-webhook -n cert-manager
```

Project landing page: <https://opscalehub.github.io/cert-manager-webhook-arvancloud>
(served from [`docs/`](docs/) via the `pages` workflow, which also hosts the Helm repo).

Key values (`charts/arvancloud-webhook/values.yaml`, validated by
`values.schema.json`):

| Value | Default | Notes |
|-------|---------|-------|
| `groupName` | `acme.arvancloud.ir` | Must match the Issuer's `webhook.groupName`. |
| `image.repository` | `ghcr.io/opscalehub/cert-manager-webhook-arvancloud` | |
| `certManager.namespace` / `certManager.serviceAccountName` | `cert-manager` | SA granted permission to call the webhook. |
| `replicaCount` | `1` | 1–10. |
| `metrics.enabled` / `metrics.port` | `true` / `8081` | Prometheus `/metrics` + `/healthz` on a separate port. |
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus-Operator `ServiceMonitor`. |

## Credentials

Create a **dedicated ArvanCloud Machine User** — never a master-account key —
and issue it an API key **scoped to DNS management for only the zones this
webhook manages**. A compromised webhook Pod has exactly the blast radius of
this token, so keep it least-privilege and **rotate it on a schedule**: create
the replacement key, update the Secret, restart the Deployment, then revoke the
old key in the ArvanCloud console.

```sh
kubectl -n cert-manager create secret generic arvancloud-credentials \
  --from-literal=api-key=<YOUR_MACHINE_USER_KEY>
```

The key is read from this namespaced Secret at request time only — it is never
written to logs, ConfigMaps, or Helm values.

## Example `ClusterIssuer`

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-arvancloud
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: platform@example.com
    privateKeySecretRef:
      name: letsencrypt-arvancloud-account
    solvers:
      - dns01:
          webhook:
            groupName: acme.arvancloud.ir
            solverName: arvancloud
            config:
              apiKeySecretRef:
                name: arvancloud-credentials
                key: api-key
              ttl: 120
              propagationCheck: true
```

A staging example plus a test `Certificate` is in
[`testdata/clusterissuer-staging.yaml`](testdata/clusterissuer-staging.yaml).

### Solver config fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `apiKeySecretRef.name` / `.key` | yes | – | Secret holding the API key, in the Issuer's namespace (or the cluster resource namespace for a `ClusterIssuer`). |
| `apiUrl` | no | `https://napi.arvancloud.ir` | Override the API base URL. |
| `ttl` | no | `120` | TXT record TTL, seconds. |
| `propagationCheck` | no | `true` | Block `Present` until `1.1.1.1` and `8.8.8.8` both serve the record. Set `false` to return as soon as the API accepts the record and rely on cert-manager's own DNS self-check. |
| `propagationTimeoutSeconds` | no | `60` | Upper bound on the propagation wait. Keep below the apiserver `--request-timeout` (60s). |

## Observability

The webhook exposes Prometheus metrics and `/healthz` on `:8081` (configurable
via `METRICS_BIND_ADDRESS`; empty disables). Metrics:

| Metric | Type | Labels |
|--------|------|--------|
| `arvancloud_webhook_api_request_duration_seconds` | histogram | `method`, `outcome` (`2xx`/`4xx`/`5xx`/`error`) |
| `arvancloud_webhook_api_requests_total` | counter | `method`, `code` |
| `arvancloud_webhook_solver_challenges_total` | counter | `action` (`present`/`cleanup`), `result` |

Enable scraping with `--set metrics.serviceMonitor.enabled=true` (requires
Prometheus Operator). Alert on rising `..._api_requests_total{code=~"429|5.."}`
and `..._challenges_total{result="error"}`.

## Development

```sh
go test ./...                     # unit tests (httptest mock API + fake k8s client)
go vet -tags conformance ./...     # keep the conformance suite compiling
go build ./...
helm lint charts/arvancloud-webhook
docker build -t webhook:dev .
```

**Conformance suite** — cert-manager's official external-webhook tests against
the live ArvanCloud API. Excluded from `go test ./...` by the `conformance`
build tag; needs envtest binaries plus `ARVANCLOUD_API_KEY` and `TEST_ZONE_NAME`:

```sh
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path 1.31.x)"
ARVANCLOUD_API_KEY=... TEST_ZONE_NAME=example.ir. \
  go test -tags conformance -run TestConformance ./internal/provider/ -v -timeout 20m
```

The `compat` workflow builds + unit-tests against a matrix of cert-manager
minor versions weekly to catch upstream API-schema breaks early, and runs the
live conformance suite on demand.

Repository layout:

```
internal/client/     ArvanCloud REST client (arvan.go, types.go) + tests
internal/obs/        Prometheus metrics + diagnostic HTTP server
internal/provider/   webhook.Solver impl + propagation check + tests + conformance
main.go              apiserver entrypoint + metrics server
charts/              Helm chart with values.schema.json + ServiceMonitor
deploy/bundle.yaml   chart rendered with defaults, for GitOps
docs/index.html      GitHub Pages landing page (dark-mode, zero-JS-framework)
.github/workflows/   ci.yml, release.yml (signed image + OCI chart), pages.yml,
                     compat.yml (cert-manager version matrix + conformance)
```

> Release note: the multi-arch pipeline uses `docker buildx` rather than
> GoReleaser; both produce `linux/amd64` + `linux/arm64` artifacts. Images and
> the OCI chart are signed keylessly with [cosign](https://docs.sigstore.dev/)
> and ship an SBOM + build provenance attestation.

## License

[Apache 2.0](LICENSE)
