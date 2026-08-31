---
title: "Custom resources"
weight: 25
---

This section describes the custom resource definitions (CRDs) that are defined 
by and used in the Chantico project. These CRDs describe the structure and 
behavior of the custom resources that are managed by the Chantico controllers. 
Currently, we have CRDs that are specifications for declaration of components 
that are part of a data center, how to connect to them for energy monitoring, 
how to obtain the energy usage metrics of such DC components, and how relations 
are formed between those devices.

The following CRDs are currently defined in the Chantico project:

- `MeasurementDevice` — [SNMP configuration](snmp-config.md): This CRD defines 
  a measurement device, which is a logical representation of a type of physical 
  device that can be monitored for energy usage. This information should be 
  usable across several instances of such a device, such as PDUs of the same 
  brand, bare metal servers deployed in a homogeneous manner, etc. It includes 
  information about the device type, its SNMP module and authentication 
  definition. The controller writes combined SNMP exporter configuration and 
  reloads the exporter instance when a `MeasurementDevice` is updated.
- `PhysicalMeasurement` — [Prometheus scrape monitoring](monitor.md): This CRD 
  defines a physical measurement device, which is a concrete data center 
  component (like a PDU or bare metal, but also a hypervisor running on that 
  bare metal) that can be monitored for energy usage. It includes information 
  about the device type, its IP address, and its relationship to 
  a `MeasurementDevice`. The controller writes Prometheus scrape configuration 
  and reloads Prometheus when a `PhysicalMeasurement` is updated.
- `DataCenterResource` — [resource graph](graph.md): This CRD defines a data 
  center resource, which can be a physical or virtual component of a data 
  center. It includes information about the resource type, its energy metrics, 
  and its relationships to other resources. The resource spec is augmented with 
  configuration data to create recording rules in Prometheus which aggregate 
  information from connected parents, see [energy 
  accounting](energy-accounting.md) for more details.

Later on, chantico may also control and manage other types of (custom) resources 
such as deployments of components part of the chantico core, to make it clearer 
how we reconfigure components when changes are made, instead of doing this from 
the controller of the subordinate components.

