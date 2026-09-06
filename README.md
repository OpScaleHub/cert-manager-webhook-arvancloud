# cert-manager-webhook-arvancloud

A production-grade [cert-manager](https://cert-manager.io) ACME **DNS-01**
webhook solver for [ArvanCloud](https://www.arvancloud.ir/) DNS, using the
ArvanCloud CDN/DNS API v4 and Machine User API keys.

It implements the official `webhook.Solver` interface
(`github.com/cert-manager/cert-manager/pkg/acme/webhook`) and runs as a small
extension-apiserver alongside cert-manager.

## Features

- **API v4** against `https://napi.arvancloud.ir`, `Authorization: Apikey <key>`.
- **Idempotent** `Present` (reuses an existing identical TXT record) and
  `CleanUp` (a missing record is success, not an error loop).
- **Hard 15s timeout** and bounded retries on every API call, so a slow
  ArvanCloud response never stalls a cert-manager queue worker.
- **Multi-level subdomain** handling (`_acme-challenge.apps.cluster.example.com`).
- Optional **public-resolver propagation check** (1.1.1.1 / 8.8.8.8) before
  `Present` returns.
- Credentials read **only** from a namespaced Kubernetes `Secret`; never logged.
- Restricted pod security context: non-root `10001`, read-only root FS, all
  capabilities dropped, `RuntimeDefault` seccomp.
- Multi-arch image (`linux/amd64`, `linux/arm64`), distroless base.

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

## Credentials

Create an ArvanCloud **Machine User** and API key, then:

```sh
kubectl -n cert-manager create secret generic arvancloud-credentials \
  --from-literal=api-key=<YOUR_MACHINE_USER_KEY>
```

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
| `propagationCheck` | no | `false` | Block `Present` until 1.1.1.1 and 8.8.8.8 both serve the record. |

## Development

```sh
go test ./...           # unit tests (httptest mock API + fake k8s client)
go build ./...
helm lint charts/arvancloud-webhook
docker build -t webhook:dev .
```

### Conformance testing

`main_test.go` runs cert-manager's own [DNS-01 conformance test
suite](https://github.com/cert-manager/cert-manager/tree/master/test/acme) --
the standard every community solver runs against the real provider API
before it's trusted with a real certificate. It spins up a local
Kubernetes control plane (via `setup-envtest`) and calls `Present`/`CleanUp`
against a real zone with a synthetic challenge -- no ACME order, no
Let's Encrypt rate limits involved.

```sh
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
ASSETS="$(~/go/bin/setup-envtest use 1.31.0 -p path)"
export TEST_ASSET_ETCD="$ASSETS/etcd"
export TEST_ASSET_KUBE_APISERVER="$ASSETS/kube-apiserver"
export TEST_ASSET_KUBECTL="$ASSETS/kubectl"

TEST_ZONE_NAME="yourzone.example." ARVANCLOUD_API_KEY=... go test -v -timeout 5m .
```

Requires a real ArvanCloud Machine User API key with DNS-records
permission actually granted for the target zone -- a Machine User
without that scope explicitly enabled gets `HTTP 403: Your access to
this section is restricted.` from every call, not a clearer
permission-denied message. Confirmed passing live against a real zone
2026-09-06.

Repository layout:

```
internal/client/     ArvanCloud REST client (arvan.go, types.go) + tests
internal/provider/   webhook.Solver implementation + propagation check + tests
main.go              apiserver entrypoint
charts/              Helm chart with values.schema.json
deploy/bundle.yaml   chart rendered with defaults, for GitOps
docs/index.html      GitHub Pages landing page (dark-mode, zero-JS-framework)
.github/workflows/   ci.yml (test/lint/helm/docker), release.yml (image + OCI chart),
                     pages.yml (landing page + gh-pages Helm repo)
```

> Release note: the multi-arch pipeline uses `docker buildx` rather than
> GoReleaser; both produce `linux/amd64` + `linux/arm64` artifacts.

## License

[Apache 2.0](LICENSE)
