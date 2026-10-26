---
title: "Energy Accounting via Prometheus Recording Rules"
weight: 35
---

This document describes the energy accounting feature implemented in the
DataCenterResource controller. It explains the design, the data model, how
Prometheus recording rules are generated, and how to test the full pipeline
end-to-end with a local kind cluster.

---

## Overview

Data center resources (PDUs, bare metals, VMs, …) form a **tree**. Energy
flows from the root nodes (PDUs whose power is measured by hardware) down to
leaf nodes (servers, VMs, pods). Each edge in the tree carries a
**coefficient** that describes what fraction of the parent's power is
attributable to the child.

Chantico turns this tree into **Prometheus recording rules** so that every
node in the tree is represented by the shared `chantico_power_watts`
timeseries. The `resource` label identifies the node, while labels such as
`type` and `parents` describe it.

### Rule types

| Rule kind | When generated | Example |
|---|---|---|
| **Alias rule** | Root node (has `energyMetricFrom` set) | `chantico_power_watts{resource="pdu1", type="pdu"} = tnoPduPowerValue{job="tno"}` |
| **Coefficient rule** | Child node, per parent with a coefficient | `chantico_power_coefficient{child="bm1", parent="pdu1"} = 1` |
| **Energy rule** | Child node (has parents) | `chantico_power_watts{resource="bm1", type="baremetal", parents="pdu1"} = sum(chantico_power_coefficient{child="bm1"} * on (parent) group_left () label_replace(chantico_power_watts, "parent", "$1", "resource", "(.*)"))` | 

---

## Data Model

### `TemplateFrom`

```go
type TemplateParameter struct {
  Name string `json:"name"`
  SecretConfigMapSelector `json:",inline,omitempty"`
}

type TemplateFrom struct {
  SecretConfigMapSelector `json:",inline,omitempty"`
  TemplateParameters []TemplateParameter `json:"templateParameters,omitempty"`
}
```

The embedded selector supplies the PromQL expression or template. Set `value` to
provide it inline, or `valueFrom` to load it from a ConfigMap or Secret. The
optional `templateParameters` field supplies named values for Go-template
placeholders. Each parameter accepts a `value` or `valueFrom`, so its value can
be provided inline or loaded from a ConfigMap or Secret. If your expression has
no placeholders, omit `templateParameters`.

When a template ConfigMap is updated, Chantico reconciles every
DataCenterResource in the same namespace that references it through
`energyMetricFrom` or `coefficientFrom`.

### `ParentRef`

```go
type ParentRef struct {
  Name            string       `json:"name"`
  CoefficientFrom *TemplateFrom `json:"coefficientFrom,omitempty"`
}
```

Each entry in `spec.parents` references a parent DataCenterResource by name
and must provide `coefficientFrom`, which supplies the PromQL expression for
that parent-child edge. A simple coefficient such as `1` is written under
`coefficientFrom.value`. If the expression comes from a ConfigMap- or Secret, use
`coefficientFrom.valueFrom`. The coefficient can also be a Go template when
`templateParameters` are provided. Examples include:

- All of a parent's energy flows to the child: use `value: "1"`.
- Part of a parent's energy flows to the child: use a fraction or PromQL
  expression.

Root resources (those without parents) must provide `energyMetricFrom` in the
same `TemplateFrom` shape. Resources with parents may also provide it, but
their energy is calculated from the parent coefficients.

### `DataCenterResourceSpec` (relevant fields)

| Field | Type | Description |
|---|---|---|
| `parents` | `[]ParentRef` | Parent resources; each entry requires a coefficient expression in `coefficientFrom` |
| `energyMetricFrom` | `TemplateFrom` | PromQL expression source for root nodes; accepts inline `value` or a `valueFrom` reference, with optional template parameters |
| `additionalLabels` | `map[string]string` | Additional labels to attach to the resource's Prometheus timeseries |

`valueFrom` supports `configMapKeyRef` and `secretKeyRef`. Template text uses Go
template syntax such as `{{ .vmid }}`; provide matching entries in
`templateParameters` to render those placeholders. Each template parameter
accepts a literal `value` or a `valueFrom` reference. See [How to register
data center resources]({{% relref "how-tos/how-to-register-data-center-resources.md" %}})
for examples.

#### Time Series Labels

The default time series labels applied to every resource are:

| Label | Description |
|---|---|
| `resource` | The name of the resource (from `metadata.name`) |
| `type` | The type of the resource (from `spec.type`) |
| `serviceId` | The service ID of the resource (from `spec.serviceId`) |
| `parents` | Comma-separated list of parent resource names (from `spec.parents`) |

The `additionalLabels` field allows you to attach arbitary identification infromation to the resource's Prometheus timeseries. These labels are merged with the default labels when generating the recording rules. For example:

```yaml
additionalLabels:
  rackNumber: "7"
```

Ensure that the keys do not conflict with the default labels, as conflicting additional labels will be skipped. 

### Example CRs

**Root node (PDU)** — has `energyMetricFrom`, no parents:

```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-pdu1
  namespace: chantico
spec:
  type: pdu
  physicalMeasurements:
    - physicalMeasurement-pdu1-out
  energyMetricFrom:
    value: 'tnoPduPowerValue{job="tno"}'
```

**Child node (bare metal)** — has parents with coefficients:

```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-misd-gbm-01
  namespace: chantico
spec:
  type: baremetal
  parents:
    - name: datacenterresource-pdu1
      coefficientFrom:
        value: "1"
    - name: datacenterresource-pdu2
      coefficientFrom:
        value: "1"
```

---

## Implementation

### File layout

| File | Purpose |
|---|---|
| `api/v1alpha1/datacenterresource_types.go` | CRD types (`ParentRef`, `DataCenterResourceSpec`) |
| `internal/datacenterresource/rules_pure.go` | Pure rule-generation logic — no I/O, no K8s dependencies |
| `internal/datacenterresource/rules_io.go` | File I/O: write / delete YAML rule files on the shared volume |
| `internal/datacenterresource/action.go` | State machine wiring: `WriteRuleFile` on entry, `DeleteRuleFile` on delete |
| `internal/datacenterresource/rules_pure_test.go` | Unit tests for rule generation |
| `internal/datacenterresource/rules_io_test.go` | Unit tests for file I/O |
| `config/initial-deployments/templates/prometheus.yaml` | Prometheus deployment with `rule_files` and `rules/` directory |

### How it works

1. When a `DataCenterResource` is created or updated, the state machine enters
   `StateEntry` which calls `WriteRuleFile`.
2. `WriteRuleFile` calls `BuildRuleFile` to generate the recording rules, then
   serialises them as YAML and writes to
   `<volume>/prometheus/rules/<name>.yml`.
3. Prometheus is configured with `rule_files: ["/tmp/prometheus-volume/rules/*.yml"]`
   and `evaluation_interval: 5s`, so it picks up new rule files automatically.
4. When a `DataCenterResource` is deleted, the `StateDelete` handler calls
   `DeleteRuleFile` to remove the YAML file, and Prometheus stops evaluating
   those rules on the next reload.

### Generated rule file example

For the bare metal `datacenterresource-misd-gbm-01` with two PDU parents:

```yaml
groups:
- name: chantico_datacenterresource_misd_gbm_01
  rules:
  - record: chantico_power_coefficient
    expr: "1"
    labels:
      child: datacenterresource-misd-gbm-01
      parent: datacenterresource-pdu1
  - record: chantico_power_coefficient
    expr: "1"
    labels:
      child: datacenterresource-misd-gbm-01
      parent: datacenterresource-pdu2
  - record: chantico_power_watts
    expr: sum(chantico_power_coefficient{child="datacenterresource-misd-gbm-01"} * on (parent) group_left () label_replace(chantico_power_watts, "parent", "$1", "resource", "(.*)"))
    labels:
      parents: datacenterresource-pdu1,datacenterresource-pdu2
      resource: datacenterresource-misd-gbm-01
      serviceId: 1ec4f74e-35bc-4e7e-aef2-9db3c94e55be
      type: baremetal

```

---

## End-to-End Testing with kind

This demo extends the standard local development environment with a second
SNMP mock PDU, so that the bare metal's energy is aggregated from two parents.

### 1. Set up the local development environment

Follow the instructions in
[How to set up the local development environment](how-tos/how-to-setup-the-local-development-environment.md).

The setup script already deploys the first SNMP mock and its
PhysicalMeasurement.

### 2. Deploy the second SNMP mock

The second mock simulates a second PDU on NodePort 31162:

```bash
kubectl apply -n chantico -f dev/k8s/snmp-mock-2-deployment.yaml
kubectl apply -n chantico -f dev/k8s/snmp-mock-2-service.yaml
```

### 3. Apply the Custom Resources for the demo

```bash
# MeasurementDevice + PhysicalMeasurement
kubectl apply -n chantico -f config/samples/chantico_v1alpha1_measurementdevice_mock.yaml
kubectl apply -n chantico -f config/samples/chantico_v1alpha1_physicalmeasurement_mock2.yaml

# DataCenterResources: PDU1, PDU2, and bare metal (BM)
kubectl apply -n chantico -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```

The sample file defines:

- **datacenterresource-pdu1** and **datacenterresource-pdu2** — root nodes whose
  `energyMetricFrom.valueFrom` references a shared ConfigMap template, with
  `templateParameters` supplying each PDU's labels.
- **datacenterresource-misd-gbm-01** — child of both PDUs, with each parent's
  coefficient supplied as `coefficientFrom.value: "1"`.

### 4. Verify the rule files

The operator writes one YAML file per DataCenterResource:

```bash
ls "$CHANTICOVOLUMELOCATIONENV/prometheus/rules/"
# Expected:
#   datacenterresource-misd-gbm-01.yml
#   datacenterresource-pdu1.yml
#   datacenterresource-pdu2.yml

cat "$CHANTICOVOLUMELOCATIONENV/prometheus/rules/datacenterresource-misd-gbm-01.yml"
```

### 5. Verify in Prometheus

Open <http://localhost:19090> and query:

```promql
chantico_power_watts
```

The result should contain one series for each configured resource: two PDU
series and one aggregated bare-metal series. All three use the same metric
name. Labels are used to identify the resource and its type.

The PDU series should contain the values supplied by the SNMP mock's
`tnoPduPowerValue` metric. To inspect only the PDU resources, query:

```promql
chantico_power_watts{type="pdu"}
```

To inspect the energy aggregated for the bare-metal resource, query it by its
`resource` label:

```promql
chantico_power_watts{resource="datacenterresource-misd-gbm-01"}
```

This value is the sum of `coefficient × parent_energy` for both PDU parents.
The coefficient-to-parent relationship is visible in the active recording
rules at <http://localhost:19090/rules>.

Other useful queries include:

```promql
# All bare-metal resources
chantico_power_watts{type="baremetal"}

# A specific PDU
chantico_power_watts{resource="datacenterresource-pdu1"}
```

### 6. Teardown

```bash
# Stop the operator (Ctrl+C in the make run terminal)
# Stop port-forwarding (Ctrl+C in the port-forward terminal)

./dev/teardown.sh
```

---

## Design Decisions

1. **Coefficients on the child, not the parent.** Each `ParentRef` in the
   child's `spec.parents` carries the coefficient for that edge. This keeps
   the parent CRs simple (they don't need to know about their children) and
   allows different children to have different coefficients for the same
   parent.

2. **File-based rule delivery.** Rules are written as YAML files to a shared
   PVC that Prometheus reads via `rule_files` glob. This avoids needing
   the Prometheus Operator or API-based rule management.

3. **Shared energy metric.** Root and child nodes use the same
  `chantico_power_watts` recording metric. The `resource` label identifies
  the node, so children can reference any parent uniformly regardless of
  whether it is a root node or an intermediate aggregation node.

4. **Pure logic + I/O separation.** `rules.go` contains only pure functions
   (no file system, no K8s client). `rules_io.go` handles the file writes.
   This makes the rule generation logic easy to unit-test.

5. **Coefficients are PromQL expressions.** The `coefficientFrom` field in
  `spec.parents` accepts an inline value, a `valueFrom` reference, or a
  templated expression using `templateParameters`. Examples include a literal
  (`"1"`, `"0.5"`) or an expression referencing an externally published
  metric. Chantico always records it under the shared `chantico_power_coefficient`
   metric with `parent` and `child` labels, so the power rule is decoupled 
   from whatever name the external source uses.

   For example, at the BM → VM level the power share per VM is determined by
   relative utilization. Computing this is **not Chantico's responsibility** —
   an external provider publishes the share as a Prometheus metric, and the VM
   resource simply references it:

   ```yaml
   spec:
     parents:
       - name: datacenterresource-misd-gbm-01
         coefficientFrom:
           value: "external_provider_power_share_vm1"
   ```

   Chantico records this as:

   ```yaml
   - record: chantico_power_coefficient
     labels:
       parent: datacenterresource-misd-gbm-01
       child: datacenterresource-vm1
     expr: external_provider_power_share_vm1
   ```

   The external provider can name its metrics freely; Chantico normalises them
   into the internal `chantico_power_coefficient` metric.
