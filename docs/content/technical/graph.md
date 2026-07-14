---
title: "Data center resource graph"
weight: 30
---

This document describes the data center resource graph managed and validated by 
the DataCenterResource controller. It explains the design, the data model, how 
the graph is constructed, validated and some of the fields that are used to 
describe the graph.

Chantico is built on the foundation that physical and virtual components that 
make up a data center can be related to each other. Each component that can be 
monitored, aggregated or allocated energy-wise is represented as 
a `DataCenterResource` custom resource. The relationships between these 
resources define how energy flows from one resource to another. Together, the 
resourcesand their relationships form a graph. Once the graph is completely 
constructed, it allows for accounting all the energy usage in a data center.

One important property of the graph is that it is represented as a directed 
acyclic graph (DAG), meaning that there are no cycles in the graph. This is 
important because it allows for complete and unambiguous assignment of power 
consumption.

## Fields and validation

| Field | Type | Description |
|---|---|---|
| `metadata.name` | `string` | The name of the resource. This is used to reference the resource in `spec.parents` of other resources. |
| `spec.type` | `string` | The type of the resource (e.g. PDU, bare metal, VM, pod) |
| `spec.physicalMeasurements` | list of `string` | The list of physical measurements that are associated with this resource. A portion of these are the metrics that are relevant for energy usage calculation of this resource. |
| `spec.energyMetrics` | `string` | Prometheus metric expression to collect energy usage |
| `spec.parents` | list of `ParentRef` | Parent resources with optional coefficients |
| `spec.serviceId` | `string` | The service ID that this resource is associated with. This is used to group resources together for energy accounting purposes. |

Some of these fields are validated by the controller to ensure that the graph is 
constructed correctly, has no ambiguous or unknown information, and that there 
are no cycles or invalid flags on the resources:

- `spec.type` must be one of the known resource types: `pdu`, `baremetal`, `vm`, 
  `kubernetes` or `heat`. We expect to include more types in the future such as 
  power feeds and input power types. Currently, if another type is used, the 
  controller will not use the resource in energy usage calculations.
- `spec.parents` must reference existing resources in the same namespace and 
  those resources must not (indirectly) refer to this resource. If a parent 
  resource does not exist, the controller will not use the resource in energy 
  usage calculations.
- `spec.serviceId` must not be set for a resource that is itself a parent of 
  another resource. This is because the service ID is used to group resources 
  that are provided to a services subscription, and this is not applicable to 
  resources that are themselves split up into other resources. If, later on, we 
  encounter a use case where this is needed, such as a PDU that is provided as 
  a service on some outlets (for rack-as-a-service) but provided as a normal 
  resource device for higher-level (baremetal, VM, kubernetes) resources, we may 
  relax or extend this field.

## Example graph

In the following graph, the nodes represent `DataCenterResource` custom 
resources and the edges represent the relationships between them. The edges are 
directed from parent to child, indicating the flow of energy from one resource 
to another.

The red nodes represent input power resources (currently not yet implemented), 
the blue nodes represent resources that are not directly part of a service 
subscription (PDUs, bare metals, VMs, kubernetes clusters that are sold per 
namespace or pod, etc.), and the green nodes represent resources that are part 
of a service subscription (bare metals, VMs, kubernetes clusters, pods, etc.).

As such, the red nodes have no parents other than upstream power inputs, and the 
green nodes have no children. The green nodes are marked with a `serviceId` in 
their corresponding resources.

The red nodes are currently not implemented with specific types and fields, and 
the graph does not yet include any output power resources such as heat reuse. As 
such, this is an initial vision for the data center resource graph that Chantico 
managed and monitors.

![](../puml/data-center-resource-graph.png)

## Energy accounting

More details on the implementation of energy accounting with Prometheus 
recording rules can be found in the [energy accounting](energy-accounting.md) 
document.
