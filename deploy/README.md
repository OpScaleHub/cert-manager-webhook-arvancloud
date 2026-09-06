# Rendered manifests

`bundle.yaml` is the Helm chart rendered with default values into the
`cert-manager` namespace, for GitOps tools (Argo CD, Flux) that consume plain
manifests. Regenerate after chart changes (CI checks it is current, tolerating
only cosmetic blank-line differences between Helm patch releases — CI pins
Helm `v3.18.4`):

```sh
helm template arvancloud-webhook charts/arvancloud-webhook \
  --namespace cert-manager > deploy/bundle.yaml
```

For anything other than the defaults, template the chart yourself with your own
`--set` / `-f values.yaml` overrides rather than editing `bundle.yaml` by hand.
