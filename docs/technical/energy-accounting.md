---
title: "Energy Accounting via Prometheus Recording Rules"
weight: 30
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
**coefficient** that describes what fraction of the parent's energy is
attributable to the child.

Chantico turns this tree into **Prometheus recording rules** so that every
node in the tree has a canonical energy timeseries
`datacenter:<name>:energy_watts` that is automatically kept up to date by
Prometheus.

### Rule types

| Rule kind | When generated | Example |
|---|---|---|
| **Alias rule** | Root node (has `energyMetric` set) | `datacenter:pdu1:energy_watts = tnoPduPowerValue{job="tno"}` |
| **Coefficient rule** | Child node, per parent with a coefficient | `coefficient_pdu1_bm01 = 1` |
| **Energy rule** | Child node (has parents) | `datacenter:bm01:energy_watts = coefficient_pdu1_bm01 * datacenter:pdu1:energy_watts + ...` |

---

## Data Model

### `ParentRef`

```go
type ParentRef struct {
    Name        string `json:"name"`
    Coefficient string `json:"coefficient,omitempty"`
}
```

Each entry in `spec.parents` references a parent DataCenterResource by name
and optionally carries a coefficient (a PromQL expression, usually a literal
number like `"1"` or `"0.5"`).

### `DataCenterResourceSpec` (relevant fields)

| Field | Type | Description |
|---|---|---|
| `parents` | `[]ParentRef` | Parent resources with optional coefficients |
| `energyMetric` | `string` | Raw Prometheus metric expression for root nodes (e.g. `tnoPduPowerValue{job="tno"}`) |

### Example CRs

**Root node (PDU)** — has `energyMetric`, no parents:

```yaml
apiVersion: chantico.ci.tno.nl/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-pdu1
  namespace: chantico
spec:
  type: pdu
  physicalMeasurements:
    - physicalMeasurement-pdu1-out
  energyMetric: tnoPduPowerValue{job="tno"}
```

**Child node (bare metal)** — has parents with coefficients, no `energyMetric`:

```yaml
apiVersion: chantico.ci.tno.nl/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-misd-gbm-01
  namespace: chantico
spec:
  type: baremetal
  parents:
    - name: datacenterresource-pdu1
      coefficient: "1"
    - name: datacenterresource-pdu2
      coefficient: "1"
```

---

## Implementation

### File layout

| File | Purpose |
|---|---|
| `api/v1alpha1/datacenterresource_types.go` | CRD types (`ParentRef`, `EnergyMetric`) |
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
  - record: coefficient_datacenterresource_pdu1_datacenterresource_misd_gbm_01
    expr: "1"
  - record: coefficient_datacenterresource_pdu2_datacenterresource_misd_gbm_01
    expr: "1"
  - record: datacenter:datacenterresource_misd_gbm_01:energy_watts
    expr: >-
      coefficient_datacenterresource_pdu1_datacenterresource_misd_gbm_01 * on()
      datacenter:datacenterresource_pdu1:energy_watts +
      coefficient_datacenterresource_pdu2_datacenterresource_misd_gbm_01 * on()
      datacenter:datacenterresource_pdu2:energy_watts
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
kubectl apply -f dev/k8s/snmp-mock-2-deployment.yaml
kubectl apply -f dev/k8s/snmp-mock-2-service.yaml
```

### 3. Apply the Custom Resources for the demo

```bash
# Second MeasurementDevice + PhysicalMeasurement
kubectl apply -f config/samples/chantico_v1alpha1_measurementdevice_mock.yaml
kubectl apply -f config/samples/chantico_v1alpha1_measurementdevice_mock2.yaml
kubectl apply -f config/samples/chantico_v1alpha1_physicalmeasurement_mock2.yaml

# DataCenterResources: PDU1, PDU2, and bare metal (BM)
kubectl apply -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```

The sample file defines:

- **datacenterresource-pdu1** — root node, `energyMetric: tnoPduPowerValue{job="tno"}`
- **datacenterresource-pdu2** — root node, `energyMetric: tnoPduPowerValue{job="tno-2"}`
- **datacenterresource-misd-gbm-01** — child of both PDUs, `coefficient: "1"` for each

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
datacenter:datacenterresource_pdu1:energy_watts
```

This should return values from the SNMP mock's `tnoPduPowerValue` metric.

Then query the aggregated energy for the bare metal:

```promql
datacenter:datacenterresource_misd_gbm_01:energy_watts
```

This should return the sum of `coefficient × parent_energy` for both PDUs.

You can also inspect the active recording rules at
<http://localhost:19090/rules>.

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

3. **Alias rules for root nodes.** Root nodes (PDUs) have their raw metric
   aliased to the canonical `datacenter:<name>:energy_watts` name so that
   children can reference any parent uniformly regardless of whether it is a
   root node or an intermediate aggregation node.

4. **Pure logic + I/O separation.** `rules.go` contains only pure functions
   (no file system, no K8s client). `rules_io.go` handles the file writes.
   This makes the rule generation logic easy to unit-test.

5. **Coefficients are PromQL expressions, not literals.** The `coefficient`
   field in `spec.parents` accepts any PromQL expression — a literal (`"1"`,
   `"0.5"`) or a reference to an externally published metric. Chantico always
   aliases it to an internal name (`coefficient_<parent>_<child>`) via a
   recording rule, so the energy rule is decoupled from whatever name the
   external source uses.

   For example, at the BM → VM level the energy share per VM is determined by
   relative utilization. Computing this is **not Chantico's responsibility** —
   an external provider publishes the share as a Prometheus metric, and the VM
   resource simply references it:

   ```yaml
   spec:
     parents:
       - name: datacenterresource-misd-gbm-01
         coefficient: "external_provider_energy_share_vm1"
   ```

   Chantico records this as:

   ```yaml
   - record: coefficient_datacenterresource_misd_gbm_01_datacenterresource_vm1
     expr: external_provider_energy_share_vm1
   ```

   The external provider can name its metrics freely; Chantico normalises them
   into the internal `coefficient_*` namespace.
