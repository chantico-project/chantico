---
title: "Prometheus monitoring configuration with physical measurement resource"
weight: 25
---

This document describes the role of the `PhysicalMeasurement` custom resource in 
the configuration of Chantico components. It explains the design and data model, 
particularly how it differs from other custom resources.

Whereas the `MeasurementDevice` custom resource describes a type of device and 
*how* it could be monitored (with SNMP authentication and walks), the 
`PhysicalMeasurement` custom resource describes the actual endpoint that is to 
be monitored, i.e., *where* the device is located. This is important to make use 
of the defined SNMP exporter authentications and modules to point at a specific 
IP address to actually obtain metrics. This is what Prometheus does to scrape 
the metrics: provide the parameters to the SNMP exporter to obtain the metrics 
from a specific device, with a selected authentication setting and module.

## Fields

| Field | Type | Description |
|---|---|---|
| `metadata.name` | `string` | The name of the resource. This is used to reference the resource in instances of other custom resource definitions. |
| `spec.ip` | ` string` | The hostname or IP address of the physical device that is to be monitored. |
| `spec.measurementDevice` | `string` | Reference to the name of the `MeasurementDevice` resource that describes the type of device and how it is to be monitored (authentication and walks). |

Note that the `ip` field may be anything that is resolvable by the SNMP exporter 
instance, including hostnames, fully qualified domain names and IP addresses. 
Hostnames are only used in mock/development/demonstration environments, where 
the SNMP exporter uses the Kubernetes domain name resolution to resolve the 
hostname to a service living in the same namespace, such as the [mock SNMP 
device](../../how-tos/how-to-run-the-mock-snmp-device.md) that is used for 
development and testing. The IP address needs to point to the monitored device 
on the subnet that the kubernetes cluster is able to reach, i.e., it requires 
that the subnet is attached and has routes defined on the cluster nodes. Often, 
such SNMP devices have their endpoints attached on a private management network, 
to avoid exposing them to the public internet and reducing IP address allocation 
for such purposes. In such cases, the management network needs to be stretched 
to the routers that the cluster nodes connect to, to allow them to access those 
devices. This is relevant during the deployment of chantico in production.
