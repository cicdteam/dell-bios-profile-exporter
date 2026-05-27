# dell-bios-profile-exporter

A Helm chart that deploys a small Prometheus-compatible exporter as a DaemonSet
to watch the BIOS **System Profile** setting on Dell PowerEdge servers. It reads
the value through the host-local `racadm` utility (the iDRAC Service Module),
exposes it as metrics, and alerts when the profile drifts from a target value.

## What it does

A DaemonSet runs a small Go exporter on each selected node. On every poll the
exporter calls `racadm` inside the host PID and mount namespaces via `nsenter`,
reads the `BIOS.SysProfileSettings.SysProfile` attribute, caches it (poll
interval default 60s), and serves it as Prometheus metrics on port 9101. A small
amount of static inventory (Service Tag, model, iDRAC version) is refreshed on a
slower interval and attached as labels.

An alert fires when the current profile drifts from the configured target
(default `PerfOptimized`), when the exporter cannot read the value, or when the
last successful read is too old. Because everything goes through the host-local
iDRAC Service Module, the exporter needs **no network access to iDRAC and no
credentials**: it talks to the local management interface the same way an
on-host administrator would.

## Architecture

```
DaemonSet pod --nsenter--> host racadm --iSM (KCS/USB-NIC)--> iDRAC --> BIOS
```

Why this way: querying iDRAC over the network with Redfish would require iDRAC
credentials stored in the cluster and network reach from worker nodes into the
management plane, both of which are commonly disallowed and were deliberately
avoided here. The `syscfg` tool (from Dell's deployment toolkit) is not generally
packaged for Ubuntu, so it is not a portable option. Host-local `racadm` talking
to the iDRAC Service Module uses the in-band channel (KCS or the internal USB
NIC) that the iDRAC Service Module already maintains on the host, so it needs no
network path to the BMC and no secrets at all. The exporter simply enters the
host namespaces and runs the same `racadm` the node administrator would run.

## Requirements

- Kubernetes 1.24+.
- Helm 3.x or 4.x.
- On each target node:
  - A Dell PowerEdge server, 12th generation (12G) or newer.
  - The iDRAC Service Module installed (`dcism`), with its daemon `dcismeng`
    running and connected to the iDRAC.
  - The `racadm` binary available on the host (default path
    `/opt/dell/srvadmin/sbin/racadm`).
- One monitoring stack already running in the cluster, either
  kube-prometheus-stack (Prometheus Operator) or k8s-victoria-metrics-stack
  (VictoriaMetrics Operator).

## Installing iSM and racadm on nodes

The exporter does not install anything on the host; the iDRAC Service Module and
`racadm` must already be present on each node. The steps below cover Ubuntu
22.04 / 24.04 and Debian 11. Exact package names vary by OMSA / iSM version, so
adjust to what your repository or downloaded `.deb` provides.

Option A - add Dell's apt repository (nodes with internet access):

```bash
# Import Dell's GPG key and add the OMSA/iSM repository, then:
sudo apt-get update
sudo apt-get install -y dcism srvadmin-idracadm8
```

Option B - download the `.deb` directly (restricted networks):

```bash
# Fetch the dcism and srvadmin-idracadm8 .deb packages from linux.dell.com
# on a machine with access, copy them to the node, then:
sudo dpkg -i dcism_*.deb srvadmin-idracadm8_*.deb
sudo apt-get install -f   # pull in any missing dependencies
```

Enable and start the iDRAC Service Module daemon:

```bash
sudo systemctl enable --now dcismeng
sudo systemctl status dcismeng
```

Verify that `racadm` can read the attribute the exporter scrapes:

```bash
racadm get BIOS.SysProfileSettings.SysProfile
# expected output similar to:
# [Key=BIOS.Setup.1-1#SysProfileSettings]
# SysProfile=PerfOptimized
```

If `racadm` lives at a different path, set `exporter.racadmPath` accordingly.

## Installing the chart

### With kube-prometheus-stack

```bash
helm install dell-bios chart/ -f examples/values-prometheus.yaml -n monitoring
```

Then verify:

- DaemonSet pods are `Running` on the selected nodes:
  `kubectl -n monitoring get pods -l app.kubernetes.io/name=dell-bios-profile-exporter -o wide`.
- The ServiceMonitor is created and picked up by Prometheus:
  `kubectl -n monitoring get servicemonitor dell-bios`, and check the target
  appears under Status -> Targets in the Prometheus UI. The
  `monitoring.additionalLabels` (for example `release: kube-prometheus-stack`)
  must match your Prometheus instance's `serviceMonitorSelector`.

### With k8s-victoria-metrics-stack

```bash
helm install dell-bios chart/ -f examples/values-victoriametrics.yaml -n monitoring
```

Then verify:

- DaemonSet pods are `Running` (same command as above).
- The VMServiceScrape is created and reconciled by the VictoriaMetrics Operator:
  `kubectl -n monitoring get vmservicescrape dell-bios`, and confirm the target
  is up in vmagent / VMAgent's targets page.

### Multi-cluster

Deploy one release per cluster from the same chart, changing only
`clusterLabel.value` so every metric carries a distinct `cluster` label. A GitOps
layout keeps one chart and a thin per-cluster values overlay:

```
clusters/prod-eu/values.yaml   -> clusterLabel.value: prod-eu
clusters/prod-us/values.yaml   -> clusterLabel.value: prod-us
```

The `cluster` label is injected through the ServiceMonitor / VMServiceScrape
relabeling, so the shared dashboard and alerts can filter by cluster. See
`examples/values-multicluster.yaml` for a complete example. During a migration
from Prometheus to VictoriaMetrics, set `monitoring.stack: both` to render both
sets of CRDs at once so the old and new stacks scrape in parallel.

## Configuration

### Monitoring stack selection

- `monitoring.stack`: which monitoring CRDs to render.
  - `prometheus` - kube-prometheus-stack (ServiceMonitor/PodMonitor +
    PrometheusRule).
  - `victoriametrics` - k8s-victoria-metrics-stack (VMServiceScrape/VMPodScrape +
    VMRule).
  - `both` - render both sets, useful during a migration.
  - `none` - render no monitoring CRDs (you configure scraping manually).
- `monitoring.scrapeType`: how metrics are collected.
  - `service` - create a Service plus a ServiceMonitor/VMServiceScrape
    (recommended).
  - `pod` - create a PodMonitor/VMPodScrape and scrape pods directly, with no
    Service.

### Node selector

The DaemonSet should run only on the nodes that actually have Dell hardware and
the iDRAC Service Module. Use `placement.nodeSelector`, `placement.tolerations`,
and `placement.affinity`.

Worker-only:

```yaml
placement:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
```

Dell-only via a hardware-vendor label you maintain:

```yaml
placement:
  nodeSelector:
    hardware-vendor: dell
```

Exclude specific nodepools with affinity:

```yaml
placement:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: nodepool
                operator: NotIn
                values: ["gpu", "edge"]
```

Tolerate a control-plane taint (if your control-plane nodes are also Dell and
you want them monitored):

```yaml
placement:
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
```

### Alerts

The chart ships three rules (rendered into a PrometheusRule and/or VMRule):

- `DellBiosSysProfileDrift` - the current profile differs from the target
  (`dell_bios_sys_profile_matches_target == 0`).
- `DellBiosRacadmFailing` - the exporter could not read the value
  (`dell_bios_racadm_success == 0`); usually iSM or `racadm` is unavailable.
- `DellBiosSysProfileStale` - the value has not been read for longer than
  `alerts.rules.stale.maxAgeMinutes`, so the metric is stale.

Each rule has its own `for` duration and `severity` under `alerts.rules`:

```yaml
alerts:
  enabled: true
  targetProfile: PerfOptimized
  rules:
    drift:
      enabled: true
      for: 15m
      severity: warning
    scrapeFailing:
      enabled: true
      for: 10m
      severity: warning
    stale:
      enabled: true
      maxAgeMinutes: 30
      for: 5m
      severity: warning
```

Route them via Alertmanager. A minimal `AlertmanagerConfig` that matches the
`warning` severity these rules emit:

```yaml
apiVersion: monitoring.coreos.com/v1alpha1
kind: AlertmanagerConfig
metadata:
  name: dell-bios
  namespace: monitoring
spec:
  route:
    receiver: dell-bios-team
    matchers:
      - name: severity
        value: warning
  receivers:
    - name: dell-bios-team
      # configure your slack/email/webhook here
```

### Grafana dashboard

Set `dashboard.enabled=true` to ship the dashboard as a ConfigMap labeled
`grafana_dashboard: "1"`, which the Grafana sidecar auto-discovers and imports.
The dashboard exposes these variables: `datasource`, `cluster`, `node`,
`profile`, and `target_profile`. The datasource type is `prometheus`; this works
unchanged against VictoriaMetrics because its datasource is PromQL-compatible.
Use `dashboard.annotations` (default `grafana_folder: "Dell Hardware"`) to place
it in a Grafana folder.

## Metrics

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `dell_bios_sys_profile_info` | gauge (=1) | `node`, `profile`, `service_tag`, `model`, `idrac_version` | Info metric; the current profile value is carried in the `profile` label. |
| `dell_bios_sys_profile_matches_target` | gauge | `node`, `target` | `1` if the current profile equals the target, else `0`. |
| `dell_bios_racadm_success` | gauge | `node` | `1` if the last poll succeeded, `0` if it failed. |
| `dell_bios_racadm_duration_seconds` | gauge | `node` | Duration of the last `racadm` call, in seconds. |
| `dell_bios_racadm_errors_total` | counter | `node`, `reason` (`timeout`, `exit_code`, `parse_error`, `nsenter_failed`) | Count of failed `racadm` calls by reason. |
| `dell_bios_last_scrape_timestamp_seconds` | gauge | `node` | Unix time of the last successful poll. |
| `dell_bios_exporter_build_info` | gauge (=1) | `version`, `go_version` | Build information for the running exporter. |

## Troubleshooting

Pod in `CrashLoopBackOff`:

- Confirm the pod actually scheduled where you expect; a `nodeSelector` that
  matches no node leaves the DaemonSet with zero pods, while a wrong one can land
  it on a non-Dell node where `racadm` is missing.
- Confirm the security context is in effect: `security.privileged: true` (or the
  `SYS_ADMIN` / `SYS_PTRACE` capabilities when privileged is off) and
  `security.hostPID: true`. Without these, `nsenter` into the host namespaces
  fails immediately.
- Confirm the iDRAC Service Module and `racadm` exist on the host at the
  configured paths (`exporter.racadmPath`, `exporter.nsenterPath`).

`dell_bios_racadm_success = 0`:

- The iDRAC Service Module is not connected to the iDRAC. Check
  `systemctl status dcismeng` on the node.
- The `racadm` path is wrong or not executable. Verify `exporter.racadmPath`.
- Reproduce on the node itself: `racadm get BIOS.SysProfileSettings.SysProfile`.
  If that fails on the host, the exporter cannot succeed either.

Empty dashboard / no data:

- The scrape labels do not match the operator's selector. Check
  `monitoring.additionalLabels` against your Prometheus
  `serviceMonitorSelector` / VictoriaMetrics scrape selectors.
- The operator's namespace selector does not include the namespace you installed
  into.
- The dashboard `cluster` variable does not match the `clusterLabel.value` you
  set, so every panel filters to nothing. Set `clusterLabel.value` per cluster.

Intermittent `DellBiosRacadmFailing`:

- The iDRAC Service Module can briefly lose its link to the iDRAC, typically
  around node reboots or iDRAC resets. Short blips are expected; raise the rule's
  `for` if they are noisy, and investigate only sustained failures.

Profile shows `Custom`:

- `Custom` means at least one System Profile sub-setting was changed away from a
  named profile's defaults, so BIOS reports a custom profile rather than
  `PerfOptimized`. Inspect the individual settings to see what differs:
  `racadm get BIOS.SysProfileSettings` on the node and compare the sub-attributes
  (CPU power management, turbo, C-states, memory frequency, etc.) against the
  target profile.

## Security

### Privileges

The exporter needs `security.privileged: true` together with
`security.hostPID: true` so it can `nsenter --target 1` into the host PID and
mount namespaces and run the host's `racadm` against the iDRAC Service Module
socket. To reduce the blast radius, set `security.privileged=false`; the chart
then drops to a targeted capability set (`SYS_ADMIN` plus `SYS_PTRACE`) instead
of full privilege. Either way the pod runs as `runAsUser: 0` because entering
the host namespaces requires root.

Under Pod Security Standards / Pod Security Admission this pod cannot run in a
`restricted` namespace; install it into a namespace labeled `privileged` (for
example your monitoring namespace), since `hostPID` and the elevated security
context are incompatible with the `baseline` and `restricted` levels.

### What is NOT required

- No network access from the cluster to iDRAC; all communication is in-band on
  the host through the iDRAC Service Module.
- No iDRAC credentials stored in Kubernetes.
- No Secrets created or mounted by the chart.

## Comparison with alternatives

| Approach | How it reads the profile | Credentials | Network to iDRAC |
| --- | --- | --- | --- |
| This chart (local `racadm` via iSM) | DaemonSet runs host `racadm` through the iDRAC Service Module | None | None |
| Network Redfish exporter | HTTPS to the iDRAC Redfish API | iDRAC username/password in the cluster | Required (worker -> management plane) |
| node_exporter textfile collector | A host cron writes a `.prom` file that node_exporter reads | None | None, but needs a host-side cron job and file plumbing you maintain yourself |

## Upgrade

```bash
helm upgrade dell-bios chart/ -f my-values.yaml
```

The monitoring CRD instances (ServiceMonitor/PodMonitor/VMServiceScrape and the
PrometheusRule/VMRule) and the alert definitions are updated in place. Before a
chart-version bump, review the changes in `values.yaml` between the old and new
versions so any renamed or removed keys in your `my-values.yaml` are reconciled.

## Uninstall

```bash
helm uninstall dell-bios -n monitoring
```

This removes the DaemonSet, Service, ServiceAccount, monitoring CR instances,
alerts, and the dashboard ConfigMap. The monitoring operators' CustomResource
**Definitions** themselves are cluster-scoped and were not created by this chart,
so they are left in place; only the CR instances this chart created are removed.

## Development

```bash
helm lint chart/
helm template chart/
helm unittest chart/
./scripts/verify.sh
```

Build the exporter image:

```bash
cd exporter
docker build --platform linux/amd64 --build-arg VERSION=0.1.5 \
  -t ghcr.io/cicdteam/dell-bios-profile-exporter:0.1.5 .
```

The chart is tested against both Helm 3.x and Helm 4.x. Under Helm 4 the
helm-unittest plugin installs with
`helm plugin install https://github.com/helm-unittest/helm-unittest --verify=false`.
