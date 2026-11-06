---
title: "Prometheus Operator integration"
weight: 15
main:
  parent: technical
---

Chantico does not write Prometheus configuration files or trigger Prometheus
reloads. Instead, it manages [Prometheus Operator](https://prometheus-operator.dev/)
custom resources and lets the operator apply them to a Prometheus instance.
This keeps Chantico focused on energy attribution and allows other monitoring
configuration to share the same Prometheus.

## Overview

| Chantico resource | Prometheus Operator resource | Purpose |
|---|---|---|
| `PhysicalMeasurement` | `ScrapeConfig` | Scrape target of one physical measurement |
| `DataCenterResource` | `PrometheusRule` | Recording rules generated for one data center resource |

Chantico creates, updates and deletes these resources during reconciliation.
Each one has the same name and namespace as the Chantico resource that it is
generated from, and carries the label `app.kubernetes.io/managed-by: chantico`.

```text
PhysicalMeasurement ──▶ ScrapeConfig ──┐
                                       ├──▶ Prometheus Operator ──▶ Prometheus
DataCenterResource  ──▶ PrometheusRule ┘
```

## Scrape configuration

For every `PhysicalMeasurement` the controller reconciles a `ScrapeConfig`
(`internal/physicalmeasurement/targets.go`). The `spec.type` of the measurement
determines its content:

| `spec.type` | Metrics path | Target |
|---|---|---|
| `snmp` | `/snmp` | `spec.ip` is passed as the `target` parameter to the SNMP exporter, set with `--snmp-exporter-address` |
| `prometheus-exporter` | `/metrics` | `spec.ip` is scraped directly |

For SNMP, relabelling moves the device address to `__param_target` and
`instance`, and replaces `__address__` with the SNMP exporter address. The
labels of the target are built from the measurement, including
`spec.additionalLabels`.

Because every measurement owns its `ScrapeConfig`, scrape requirements can
differ per measurement and there is no shared scrape configuration file to
maintain. There is a one to one relationship between a `PhysicalMeasurement` 
and its `ScrapeConfig`, so the `ScrapeConfig` is deleted when the 
`PhysicalMeasurement` is deleted.

## Recording rules

For every `DataCenterResource` the controller reconciles a `PrometheusRule`
containing the generated recording rules (`internal/datacenterresource/rules_writer.go`).
See [Energy accounting](custom-resources/energy-accounting.md) for how the rules
are generated. The `PrometheusRule` is deleted when the resource is deleted.

## Prometheus instance

The Chantico chart (`chart/templates/prometheus.yaml`) creates (in the Helm Chart) 
and uses a single `Prometheus` resource, with its own ServiceAccount and ClusterRole, 
that only selects the resources managed by Chantico:

```yaml
ruleSelector:
  matchLabels:
    app.kubernetes.io/managed-by: chantico
ruleNamespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: <release namespace>
scrapeConfigSelector:
  matchLabels:
    app.kubernetes.io/managed-by: chantico
scrapeConfigNamespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: <release namespace>
```

The retention, scrape and evaluation intervals, and the VictoriaMetrics
`remoteWrite` (when `victoriaMetrics.include` is enabled) are set on the same
resource. The `chantico-prometheus` Service routes to the Prometheus pods
created by the operator.

## Grafana

Grafana is deployed by the `kube-prometheus-stack` dependency instead of by
Chantico. Chantico provides its dashboards and datasources as labelled
`ConfigMap`s (`grafana_dashboard: chantico`, `grafana_datasource: chantico`)
that the Grafana sidecars load. The `chantico-grafana` Service is kept as a
stable endpoint and selects the Grafana pods. Grafana settings are configured
under `kube-prometheus-stack.grafana` in the chart values.

## Permissions

The Chantico controller needs permission to manage `scrapeconfigs` and
`prometheusrules` in the `monitoring.coreos.com` API group (`get`, `list`,
`watch`, `create`, `update`, `patch` and `delete`) in the namespaces it watches.

## Installation

The Prometheus Operator and its CRDs are installed through the
`kube-prometheus-stack` chart dependency, see
[How to install Chantico](../how-tos/how-to-install-chantico.md).
