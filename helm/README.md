# Odigos Helm Chart

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![Release](https://github.com/odigos-io/odigos/actions/workflows/release.yml/badge.svg?branch=main)](https://github.com/odigos-io/odigos/actions/workflows/release.yml)

This repository contains the helm chart for Odigos - the observability control plane.

The chart is published as an OCI artifact to `oci://odigosdemo.jfrog.io/helmoci`.

## Usage

### Install Odigos

```sh
helm upgrade --install odigos oci://odigosdemo.jfrog.io/helmoci/odigos --namespace odigos-system --create-namespace
```

- **Openshift Clusters** - Make sure to set `openshift.enabled=true` in the values file or pass it as a flag while installing the chart.
- **GKE Clusters** - Make sure to set `gke.enabled=true` in the values file or pass it as a flag while installing the chart.

#### Using a Custom Docker Registry

By default, images are pulled from odigos registry at `registry.odigos.io`. To use a custom Docker registry instead, set the `imagePrefix` value during installation:

```sh
helm upgrade --install odigos oci://odigosdemo.jfrog.io/helmoci/odigos \
  --namespace odigos-system \
  --create-namespace \
  --set imagePrefix=$CUSTOM_DOCKER_REGISTRY
```
Make sure to replace `$CUSTOM_DOCKER_REGISTRY` with the URL of your Docker registry.

For more details on configuring a custom Docker registry, refer to the [Docker Registry Setup Documentation](https://docs.odigos.io/setup/docker-registry).

### Uninstall Odigos

```sh
helm delete odigos -n odigos-system
kubectl delete ns odigos-system
```

## License

[Apache 2.0 License](https://github.com/prometheus-community/helm-charts/blob/main/LICENSE).
