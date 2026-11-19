

---
title: "System architecture"
weight: 18
main:
  parent: technical
---

The system overview shows the main components of Chantico, while the SNMP lifecycle below explains how a generated configuration reaches the exporter.

## System overview

Start with the three custom resources above the controller and follow the flow downward. 
Solid arrows show configuration or metric flow and dashed arrows show reconciliation triggers or lifecycle actions.
Green highlights SNMP configuration and export, the generation lifecycle is detailed in the next diagram.

```kroki {_type=d2 _output=embed}
direction: down

classes: {
  resource: {
    style: {
      fill: "#ffffff"
      stroke: "#94a3b8"
      font-color: "#263238"
      font-size: 20
    }
  }
  config: {
    shape: page
  }
  action: {
    style: {
      stroke: "#64748b"
      stroke-dash: 4
    }
  }
}

measurement_device: "MeasurementDevice\n<crd>" { class: resource }
physical_measurement: "PhysicalMeasurement\n<crd>" { class: resource }
datacenter_resource: "DataCenterResource\n<crd>" { class: resource }
templates: "PromQL Templates\n<cm>" { class: [resource; config] }
controller: "Chantico Controller\n<pod>" {
  class: resource
  style: {
    fill: "#e8f0fb"
    stroke: "#326CE5"
    font-color: "#263238"
    bold: true
  }
}
snmp: "" {
  grid-columns: 2
  horizontal-gap: 100
  style.fill: transparent
  style.stroke: transparent

  configuration: "SNMP configuration\n+ exporter" {
    class: resource
    style: {
      fill: "#e5f4ed"
      stroke: "#2f6d5a"
      font-color: "#214c3f"
      bold: true
      border-radius: 8
    }
  }
  file_browser: "File Browser\n<pod>" { class: resource }
}
prometheus: "Prometheus\n<pod>" { class: resource }
victoria_metrics: "VictoriaMetrics\n<pod>" { class: resource }

measurement_device -> controller: "Reconciles" { class: action }
physical_measurement -> controller: "Reconciles" { class: action }
datacenter_resource -> controller: "Reconciles" { class: action }
templates -> controller: "Reads"
controller -> snmp: { style.opacity: 0 }
controller -> snmp.configuration: "Reconciles" { class: action }
snmp.file_browser -> snmp.configuration: "MIBs"
snmp.configuration -> prometheus: "Scraped metrics"
snmp -> prometheus: { style.opacity: 0 }
controller -> prometheus: "Targets / rules"
prometheus -> victoria_metrics: "Remote write"
```

The three custom resources drive different parts of the configuration:

| Resource | Controller output |
| --- | --- |
| `MeasurementDevice` | SNMP generator input, Job, and exporter configuration |
| `PhysicalMeasurement` | `ScrapeConfig` describing a scrape target |
| `DataCenterResource` | `PrometheusRule` with energy-accounting recording rules |

PromQL template ConfigMaps are optional inputs for `DataCenterResource` expressions. 
Changes to a directly referenced template ConfigMap trigger reconciliation of the referring resources; the controller reads these ConfigMaps.

## SNMP configuration lifecycle

Read from top to bottom for step-by-step SNMP configuration procedure. Each arrow points from the participant performing an action to its target. 
The ConfigMaps participant represents the three distinct configurations listed below the diagram.
The Job reads a mounted ConfigMap; it does not write ConfigMaps or need Kubernetes API credentials.

```kroki {_type=d2 _output=embed}
shape: sequence_diagram
controller: "Chantico controller" {
  style.fill: "#e8f0fb"
  style.stroke: "#326CE5"
}
configmaps: "ConfigMaps" {
  style.fill: "#ffffff"
  style.stroke: "#94a3b8"
}
generator: "Generator Job" {
  style.fill: "#ffffff"
  style.stroke: "#94a3b8"
}
volume: "Shared data PVC" {
  style.fill: "#ffffff"
  style.stroke: "#94a3b8"
}
exporter: "SNMP Exporter" {
  style.fill: "#ffffff"
  style.stroke: "#94a3b8"
}
controller -> configmaps: "1. Write generator input"
controller -> generator: "2. Create Job"
generator -> configmaps: "3. Read mounted input"
generator -> volume: "4. Read MIBs"
generator -> volume: "5. Write generated file"
generator -> controller: "6. Job completion observed"
controller -> volume: "7. Read and validate output"
controller -> configmaps: "8. Store per-device SNMP fragment"
controller -> volume: "9. Remove persisted output"
controller -> configmaps: "10. Read fragments and write merged config"
controller -> exporter: "11. Reload exporter"
exporter -> configmaps: "12. Mount merged config"
```

### Configuration and storage

| Configuration | ConfigMap | Written by | Read by |
| --- | --- | --- | --- |
| Generator input | `<device>-generator` | Controller | Generator Job, through its read-only mount |
| Per-device SNMP fragment | `<device>-snmp` | Controller | Controller, when merging |
| Merged SNMP config | `chantico-snmp-config` | Controller | SNMP exporter, through its read-only mount |

1. The file browser manages MIBs in `snmp/mibs/` on the shared data PVC. The generator mounts that directory read-only.
2. The Job writes to `snmp/output/<device-uid>/<generation>.yml` on the same PVC. The device UID and generation keep results separate.
3. After Job completion, the controller validates the file and persists the fragment ConfigMap before deleting the temporary output. 
   Missing or invalid output does not replace the previous fragment; a failed ConfigMap write leaves the file available for retry.
4. The controller merges the fragments and triggers an exporter rollout when the merged configuration changes. The exporter mounts only the merged ConfigMap, not the shared PVC or the individual fragments.