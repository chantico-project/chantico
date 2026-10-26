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
resources and their relationships form a graph. Once the graph is completely 
constructed, it allows for accounting all the energy usage in a data center.

One important property of the graph is that it is represented as a directed 
acyclic graph (DAG), meaning that there are no cycles in the graph. This is 
important because it allows for complete and unambiguous assignment of power 
consumption.

## Fields and validation

| Field | Type | Description |
|---|---|---|
| `metadata.name` | `string` | The name of the resource. This is used to reference the resource in `spec.parents` of other resources. |
| `spec.type` | `string` | The resource category, listed under [Resource types](#resource-types). |
| `spec.physicalMeasurements` | list of `string` | The list of physical measurements that are associated with this resource. A portion of these are the metrics that are relevant for energy usage calculation of this resource. |
| `spec.energyMetric` | `string` | Prometheus expression providing the power timeseries for a root resource. |
| `spec.energyMetricFrom` | `TemplateFrom` | ConfigMap-backed template for the root resource's power expression. |
| `spec.parents` | list of `ParentRef` | Parent resources, each with a coefficient or coefficient template. |
| `spec.serviceId` | `string` | The service ID that this resource is associated with. This is used to group resources together for energy accounting purposes. |

These fields are validated by the API schema and controller to ensure that the
graph is constructed correctly, has no ambiguous or unknown information, and
that there are no cycles or invalid flags on the resources:

- `spec.type` must be one of the resource types listed below. The controller
  rejects unknown types and does not use them in energy usage calculations.
- `spec.parents` must reference existing resources in the same namespace and 
  those resources must not (indirectly) refer to this resource. If a parent 
  resource does not exist, the controller will not use the resource in energy 
  usage calculations. Each parent reference must specify either `coefficient`
  or `coefficientFrom`, but not both.
- `spec.energyMetric` and `spec.energyMetricFrom` are mutually exclusive. A
  resource with no parents must specify one of them, including power sources
  such as grids, generators and batteries.
- `spec.serviceId` must not be set for a resource that is itself a parent of 
  another resource. This is because the service ID is used to group resources 
  that are provided to a service subscription, and this is not applicable to
  resources that are themselves split up into other resources. If, later on, we 
  encounter a use case where this is needed, we may relax or extend this field.
  This restriction also applies to outlets: an outlet can have a `serviceId`
  only when it has no children.

## Resource types

A resource type describes what a graph node represents, not how it is monitored.
The collection method, such as SNMP or a Prometheus exporter, belongs to the
`PhysicalMeasurement`. The resource type is also published as the `type` label
on the resource's power timeseries.

| Type | Meaning |
|---|---|
| `grid` | An electricity grid connection supplying the data center. |
| `solar` | Solar generation directly connected to the data center, from a single installation or an aggregate source. |
| `wind` | Wind generation directly connected to the data center, from one or more turbines. |
| `generator` | An engine-driven electrical generator, such as a diesel generator. The type does not prescribe a particular fuel. |
| `battery` | Battery energy storage that can supply the data center. |
| `power-feed` | A supply path connecting one or more power sources to all or part of the data center. |
| `outlet` | An individual PDU outlet supplying a connected resource or service, not an entire PDU. |
| `baremetal` | A physical server, distinct from the virtual workloads running on it. |
| `hypervisor` | The virtualization layer hosting VMs on a bare-metal server, represented separately from the physical host. |
| `vm` | A virtual machine. |
| `kubernetes` | A Kubernetes cluster whose parent resources can be bare-metal servers or VMs, not an individual pod. |
| `heat` | A heat output or heat reuse resource. |

For the initial power-feed model, resources connected to a feed are assumed to
be supplied by all sources referenced by that feed. Declare these relationships
through `spec.parents`; the type does not automatically create connections.

The existing `kubernetes` value denotes a cluster. There is no separate
`kubernetes-cluster` or pod type. Hypervisors and clusters provide intermediate
nodes for distinguishing whole-server power from the power attributed to hosted
workloads.

The former `pdu` value has been replaced by `outlet` and is no longer accepted.
Migrate outlet-level resources to `outlet` and update dashboard queries that
select `type="pdu"`. Whole-PDU measurements should not simply be relabeled as
individual outlets.

## Example graph

In the following graph, the nodes represent `DataCenterResource` custom 
resources and the edges represent the relationships between them. The edges are 
directed from parent to child, indicating the flow of energy from one resource 
to another.

The red nodes represent power sources and power feeds,
the blue nodes represent resources that are not directly part of a service 
subscription (PDUs, bare metals, VMs, Kubernetes clusters that are sold per
namespace or pod, etc.), and the green nodes represent resources that are part 
of a service subscription (bare metals, VMs, Kubernetes clusters, pods, etc.).

As such, the red nodes have no parents other than upstream power inputs, and the 
green nodes have no children. The green nodes are marked with a `serviceId` in 
their corresponding resources.

This diagram is a conceptual view rather than a literal resource manifest. Its
PDU nodes represent physical distribution equipment; `outlet` identifies
individual PDU connections in the resource model. Pods illustrate a future
extension and do not currently have their own resource type. Solar, wind,
hypervisor and heat resources are not shown.

![](../../puml/data-center-resource-graph.png)

## Energy accounting

Accounting remains generic: a resource's type does not select a different
aggregation algorithm. Root resources provide a power timeseries, and child
resources derive their power from their parents and the coefficients on those
relationships. The new types remain subject to the same validation rules.

New algorithms for source allocation and overhead calculation are outside the
scope of [issue #200](https://github.com/chantico-project/chantico/issues/200).
Adding resource types does not itself implement those calculations or automatic
discovery of resources.

More details on the implementation of energy accounting with Prometheus 
recording rules can be found in the [energy accounting](energy-accounting.md) 
document.
