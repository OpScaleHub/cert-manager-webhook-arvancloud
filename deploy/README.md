# Rendered manifests (GitOps)

Argo CD / Flux users normally point straight at the Helm chart
(`charts/arvancloud-webhook`) and let the GitOps controller render it.

If you need **plain, pre-rendered manifests** instead:

- **Per release:** every [GitHub Release](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/releases)
  attaches a `bundle.yaml` rendered from that exact chart version.
- **Ad hoc / customised:** render it yourself with your own values —

  ```sh
  helm template arvancloud-webhook \
    oci://ghcr.io/opscalehub/charts/arvancloud-webhook --version <VERSION> \
    --namespace cert-manager \
    -f my-values.yaml > bundle.yaml
  ```

A rendered bundle is intentionally **not** committed to the repo: it is
version-stamped output and would drift from `Chart.yaml` on every release.
