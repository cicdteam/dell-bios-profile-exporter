# dell-bios-profile-exporter

[![CI](https://github.com/cicdteam/dell-bios-profile-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/cicdteam/dell-bios-profile-exporter/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/cicdteam/dell-bios-profile-exporter)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/cicdteam/dell-bios-profile-exporter?filename=exporter/go.mod)](exporter/go.mod)
[![Release](https://img.shields.io/github/v/release/cicdteam/dell-bios-profile-exporter?sort=semver)](https://github.com/cicdteam/dell-bios-profile-exporter/releases/latest)

A Prometheus-compatible exporter and Helm chart that monitor the BIOS
**System Profile** attribute on Dell PowerEdge servers via the host-local
`racadm` utility (through the iDRAC Service Module), and alert when it drifts
from a target value (default `PerfOptimized`).

## Contents

- `exporter/` - the Go exporter (DaemonSet container).
- `chart/` - the Helm chart (DaemonSet, monitoring CRDs, alerts, dashboard).
- `examples/` - ready-made values for common setups.
- `scripts/verify.sh` - runs lint/template/unit/kubeconform checks.

## Build the image

```bash
cd exporter
docker build --platform linux/amd64 --build-arg VERSION=0.1.1 \
  -t ghcr.io/cicdteam/dell-bios-profile-exporter:0.1.1 .
```

## Test the chart without installing

The chart works with both Helm 3.x and Helm 4.x.

```bash
helm lint chart/
helm template chart/
helm unittest chart/
./scripts/verify.sh
```

Note: under Helm 4 the helm-unittest plugin installs with
`helm plugin install https://github.com/helm-unittest/helm-unittest --verify=false`.

## Install in an air-gapped environment

```bash
# On a connected host, pull the published chart from the OCI registry:
helm pull oci://ghcr.io/cicdteam/charts/dell-bios-profile-exporter --version 0.1.1
# copy dell-bios-profile-exporter-0.1.1.tgz into the closed network, then:
helm install dell-bios ./dell-bios-profile-exporter-0.1.1.tgz -f my-values.yaml
```
The container image must be mirrored into the private registry separately
(for example with `docker save` / `skopeo copy` into your internal registry).

See `chart/README.md` for detailed usage. Russian: `README.rus.md`.
