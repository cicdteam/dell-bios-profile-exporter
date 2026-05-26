#!/usr/bin/env bash
set -euo pipefail

CHART=chart
HELM4="${HELM4:-helm}"
HELM3="${HELM3:-$HOME/bin/helm3}"
PROM_API="--api-versions monitoring.coreos.com/v1"
VM_API="--api-versions operator.victoriametrics.com/v1beta1"

echo "==> jq dashboard"
jq . "$CHART/dashboards/dell-bios-profile.json" >/dev/null

for HELM in "$HELM4" "$HELM3"; do
  echo "############################################"
  echo "## helm binary: $HELM ($($HELM version --short 2>/dev/null))"
  echo "############################################"

  echo "==> helm lint"
  "$HELM" lint "$CHART"

  render() {
    echo "==> helm template $*"
    # shellcheck disable=SC2086
    "$HELM" template "$CHART" "$@" | kubeconform -strict -ignore-missing-schemas -summary
  }

  render
  render -f examples/values-prometheus.yaml $PROM_API
  render -f examples/values-victoriametrics.yaml $VM_API
  render -f examples/values-multicluster.yaml $PROM_API $VM_API
  render --set monitoring.stack=both $PROM_API $VM_API
  render --set monitoring.stack=none
  render --set monitoring.scrapeType=pod $PROM_API
  render --set alerts.enabled=false $PROM_API
  render --set dashboard.enabled=true
  render --set security.privileged=false
done

echo "==> helm unittest (helm4 plugin)"
"$HELM4" unittest "$CHART"

echo "==> ALL CHECKS PASSED"
