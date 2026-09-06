# Changelog

## [1.0.1](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/compare/v1.0.0...v1.0.1) (2026-09-06)


### Bug Fixes

* **release:** cosign chart signing needs docker login; add workflow_dispatch tag re-publish ([e504d00](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/e504d00a939dd5406e815975f68c0ffea7cdec11))


### Documentation

* note release-please PR check approval + bundle.yaml release asset ([c284f8f](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/c284f8ff8f97e5dae5d51848d891c658819a3c34))
* packages are public; document manual re-publish path ([5045222](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/504522221ef96b2cf2c4bbfdb9d02af78fb7cca9))


### Build & CI

* stop committing deploy/bundle.yaml (drifts on every release) ([4392893](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/43928931d51f268abe1720112245a559df0bae6c))

## 1.0.0 (2026-09-06)


### Features

* **docs:** real OG image + favicons; chore(client): shared pooled HTTP transport ([c77c42a](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/c77c42aa504387ce97ef7cd983c537521326dbe8))
* initial ArvanCloud cert-manager DNS-01 webhook ([5a61238](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/5a61238653b06137137443070e91885243d19465))
* metrics, propagation-on-by-default, signing, cert-manager compat matrix ([7c46bea](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/7c46beaac11aaafcd9e7527ae6d499c3d728530b))


### Bug Fixes

* **ci:** golangci-lint v2, pin Helm, resilient bundle check; align to v1.0.0 ([4891f85](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/4891f8534842744ca5113af6d0a8243524cbc2ae))
* **client:** tolerate "Apikey " prefix in the API key; link ArvanCloud API docs ([91c5806](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/91c580682a053a182622aff96d1bcec214e7bf75))
* **provider:** default propagation timeout 45s (headroom under 60s apiserver request-timeout) ([baba180](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/baba18084de620ec6a8b61a7fe980b4beb019705))
* **release:** id-token perm for keyless cosign, lowercase GHCR refs ([c181d80](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/c181d802d8892990eec100f1dbcad1b9d6278ac8))


### Build & CI

* automate releases with release-please ([eb99cf6](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/eb99cf69cbcdc56f43158b47dfe5e03e95e40ec3))
* fix release-please updaters (yaml jsonpath for Chart.yaml, clean CHANGELOG stub) ([9651d16](https://github.com/OpScaleHub/cert-manager-webhook-arvancloud/commit/9651d160447ca2833d5088c3995f72837f2a386a))
