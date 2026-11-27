---
title: "Prometheus Operator integration"
weight: 15
main:
  parent: technical
---


Chantico uses the [Prometheus Operator](https://prometheus-operator.dev/) 
to handle configuration of the Prometheus instance. Chantico manages the relevant
custom resources and lets the operator apply them to a Prometheus instance.
This keeps Chantico focused on energy attribution and allows other monitoring
configuration to share the same Prometheus.

## Overview

| Chantico resource | Prometheus Operator resource | Purpose |
|---|---|---|
| `PhysicalMeasurement` | `ScrapeConfig` | Scrape target of one physical measurement |
| `PhysicalMeasurement` | `PrometheusRule` | Health alerts of one physical measurement |
| `DataCenterResource` | `PrometheusRule` | Recording rules and health alerts generated for one data center resource |

Chantico creates, updates and deletes these resources during reconciliation.
Each one has the same name and namespace as the Chantico resource that it is
generated from, except the `PrometheusRule` of a `PhysicalMeasurement`, which 
is named `physicalmeasurement-<name>`. All of them carry the labels 
`app.kubernetes.io/managed-by: chantico` and `app.kubernetes.io/part-of: chantico`.

```kroki {_type=d2 _output=embed}
direction: right

PhysicalMeasurement -> ScrapeConfig -> Operator -> Prometheus
DataCenterResource -> PrometheusRule -> Operator

Operator: "Prometheus Operator"
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
    app.kubernetes.io/part-of: chantico
ruleNamespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: <release namespace>
scrapeConfigSelector:
  matchLabels:
    app.kubernetes.io/part-of: chantico
scrapeConfigNamespaceSelector:
  matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
        - <watch namespace>
        - <release namespace>
```

The retention, scrape and evaluation intervals, and the VictoriaMetrics
`remoteWrite` (when `victoriaMetrics.include` is enabled) are set on the same
resource. The `chantico-prometheus` Service routes to the Prometheus pods
created by the operator.

The chart also adds a `ScrapeConfig` named `chantico-prometheus-self` in the 
release namespace, with which Prometheus scrapes its own metrics. It keeps only 
the metrics that the health alerts need, such as 
`prometheus_target_scrape_pool_targets` and 
`prometheus_rule_evaluation_failures_total`. The `remoteWrite` drops the series 
of this job, so they are not stored in VictoriaMetrics.

## Health tracking

The health alerts in the generated `PrometheusRule`s set conditions on the 
Chantico resources, so that problems in Prometheus are visible on the resource 
itself:

| Resource | Condition | Alerts |
|---|---|---|
| `PhysicalMeasurement` | `ScrapeConfigLoaded` | `ChanticoScrapeConfigNotLoaded` |
| `PhysicalMeasurement` | `EndpointReachable` | `ChanticoEndpointUnreachable` |
| `DataCenterResource` | `RecordingRuleHealthy` | `ChanticoRecordingRuleNoData`, `ChanticoRecordingRuleFailing` |

```kroki {_type=d2 _output=embed}
direction: right

Prometheus -> Alertmanager: alerts
Alertmanager -> Controller: "webhook (mTLS)"
Controller -> "Chantico resource": "status condition"

Controller: "Chantico controller"
```

The alerts carry the labels `chantico_kind`, `chantico_name`, `namespace` and 
`chantico_condition`, which identify the condition of a resource. The chart 
deploys an `Alertmanager` resource and an `AlertmanagerConfig` named 
`chantico-health`, which groups alerts by these labels and sends them, including 
resolved alerts, to the webhook `/alertmanager/health` of the controller through 
the `chantico-controller-webhook` Service.

The controller serves this webhook on the controller-runtime webhook server 
(port 9443). When it cannot start, for example because its certificates are 
missing, the controller logs this and keeps running without it. When the chart 
value `alertWebhook.hostAddress` is set, the Service has no selector and an 
`EndpointSlice` routes it to that address instead, for a controller that runs 
outside the cluster. The chart generates a self-signed CA with a server and a 
client certificate in the Secrets `chantico-webhook-server-cert` and 
`chantico-webhook-client-cert`, and reuses them on upgrades. The webhook server 
only accepts clients with a certificate signed by this CA. A controller outside 
the cluster uses the server certificate from a directory set with 
`--webhook-cert-dir`.

The condition lifecycle is:

1. The controller sets a missing health condition to `True` after it applied 
  the `ScrapeConfig` or `PrometheusRule` of the resource. It never changes an 
  existing health condition.
1. When an alert fires, the webhook sets the condition to `False` with reason 
  `AlertFiring` and the alert summaries as message.
1. When all alerts of the condition are resolved, the webhook sets it to `True` 
  with reason `AlertResolved`.

Health conditions are informational: they don't affect the `Ready` condition 
and don't trigger reconciliation retries. Unknown or deleted resources in 
alerts are ignored.

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
The webhook needs permission to patch the status of `physicalmeasurements` and 
`datacenterresources`.

## Installation

The Prometheus Operator and its CRDs are installed through the
`kube-prometheus-stack` chart dependency, see
[How to install Chantico](../how-tos/how-to-install-chantico.md).
