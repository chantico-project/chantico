

---
title: "System architecture"
weight: 18
main:
  parent: technical
---

This diagram summarizes the current design of the relationships between
Chantico resources, the controller, supporting services, and persistent
storage. 

## Reading the diagram

The node labels identify Kubernetes resource types: custom resources (`crd`),
ConfigMaps (`cm`), pods, jobs, and persistent volume claims (`pvc`). Arrows
describe the relationship or data flow named on each edge, such as a controller
reconciling a resource.

## Persistent volumes

The PVCs in the diagram show which storage resources are involved and which
components access them. The MIBs PVC is shown in relation to the SNMP generator, 
SNMP exporter, and file browser, while Prometheus and VictoriaMetrics each 
use storage for time-series data.

```kroki {_type=d2 _output=embed}
direction: down

classes: {
  k8s-pod: {
    shape: hexagon
    style: {
      fill: "#326CE5"
      stroke: "#1d4ed8"
      font-color: "#ffffff"
      bold: true
      font-size: 13
    }
  }
  k8s-crd: {
    shape: hexagon
    style: {
      fill: "#eff6ff"
      stroke: "#326CE5"
      stroke-width: 2
      font-color: "#1e3a8a"
      bold: true
      font-size: 13
    }
  }
  k8s-pvc: {
    shape: cylinder
    style: {
      fill: "#326CE5"
      stroke: "#1d4ed8"
      font-color: "#ffffff"
      bold: true
      font-size: 13
    }
  }
  k8s-cm: {
    shape: page
    style: {
      fill: "#dbeafe"
      stroke: "#326CE5"
      stroke-width: 2
      font-color: "#1e3a8a"
      bold: true
      font-size: 13
    }
  }
  k8s-job: {
    shape: step
    style: {
      fill: "#326CE5"
      stroke: "#1d4ed8"
      font-color: "#ffffff"
      bold: true
      font-size: 13
    }
  }
  group-box: {
    style: {
      fill: "#f8fafc"
      stroke: "#94a3b8"
      stroke-dash: 4
      border-radius: 8
      font-color: "#334155"
      bold: true
    }
  }
}

# --- Custom Resources & Template ---
measurement_device: "MeasurementDevice\n<crd>" { class: k8s-crd }
physical_measurement: "PhysicalMeasurement\n<crd>" { class: k8s-crd }
datacenter_resource: "DataCenterResource\n<crd>" { class: k8s-crd }
template: "Template\n<cm>" { class: k8s-cm }

# --- Controller ---
chantico_controller: "Chantico Controller\n<pod>" { class: k8s-pod }

# --- SNMP, MIBs & File Explorer ---
merged_snmp_config: "Merged SNMP Config\n<cm>" { class: k8s-cm }
snmp_exporter: "SNMP Exporter\n<pod>" { class: k8s-pod }
snmp_config: "SNMP Config\n<cm>" { class: k8s-cm }
snmp_generator_job: "SNMP Generator Job\n<job>" { class: k8s-job }
mibs_pvc: "MIBs\n<pvc>" { class: k8s-pvc }
file_explorer: "File Explorer\n<pod>" { class: k8s-pod }

# --- Prometheus Operator Group ---
prometheus_group: "Prometheus Operator" {
  class: group-box

  prometheus_tsd: "Prometheus TSD\n<pvc>" { class: k8s-pvc }
  prometheus: "Prometheus\n<pod>" { class: k8s-pod }
  scrape_config: "ScrapeConfig\n<crd>" { class: k8s-crd }
  prometheus_rule: "PrometheusRule\n<crd>" { class: k8s-crd }

  prometheus -> prometheus_tsd: "ReadWrite"
  scrape_config -> prometheus
  prometheus_rule -> prometheus
}

# --- Victoria Metrics Group ---
victoria_group: "Victoria Metrics" {
  class: group-box

  vm_rules: "Rules\n<cm>" { class: k8s-cm }
  vm_tsd: "Victoria Metrics TSD\n<pvc>" { class: k8s-pvc }
  victoria_metrics: "Victoria Metrics\n<pod>" { class: k8s-pod }

  victoria_metrics -> vm_rules: "ReadWrite"
  victoria_metrics -> vm_tsd: "ReadWrite"
}

# --- Relationships ---
measurement_device -> chantico_controller: "Reconciles"
physical_measurement -> chantico_controller: "Reconciles"
datacenter_resource -> chantico_controller: "Reconciles"
template -> chantico_controller: "Reconciles"

chantico_controller -> merged_snmp_config: "Writes"
snmp_config -> chantico_controller: "Reads"

merged_snmp_config -> snmp_exporter: "Reads"
snmp_config -> snmp_exporter: "Reads"
mibs_pvc -> snmp_exporter: "Reads"

snmp_generator_job -> snmp_config: "Writes"
snmp_generator_job -> mibs_pvc: "Read"
file_explorer -> mibs_pvc: "ReadWrite"

chantico_controller -> prometheus_group.scrape_config: "Writes"
chantico_controller -> prometheus_group.prometheus_rule: "Writes"
prometheus_group.prometheus -> victoria_group.victoria_metrics: "Remote Write"

```