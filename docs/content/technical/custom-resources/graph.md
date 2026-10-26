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
| `spec.energyMetricFrom` | `TemplateFrom` | Prometheus expression source for a root resource, supplied by `value` or `valueFrom` |
| `spec.parents` | list of `ParentRef` | Parent resources; each entry requires a coefficient expression in `coefficientFrom` |
| `spec.additionalLabels` | map of `string` to `string` | Any additional fields to associate with the resource. This can be used to group resources together for energy accounting purposes. |

These fields are validated by the API schema and controller to ensure that the
graph is constructed correctly, has no ambiguous or unknown information, and
that there are no cycles or invalid flags on the resources:

- `spec.type` must be one of the resource types listed below. The controller
  rejects unknown types and does not use them in energy usage calculations.
- `spec.parents` must reference existing resources in the same namespace and 
  those resources must not (indirectly) refer to this resource. If a parent 
  resource does not exist, the controller will not use the resource in energy 
  usage calculations.
- Each entry in `spec.parents` must include `coefficientFrom`. Root resources
  (those without parents) must include `energyMetricFrom`. Both fields accept
  either an inline `value` or a `valueFrom` reference; Go-template placeholders
  can be supplied through `templateParameters`.

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
their corresponding `additionalLabels` fields of their resources, to indicate 
they form client-facing services with any resource with the same ID.

This diagram is a conceptual view rather than a literal resource manifest. Its
PDU nodes represent physical distribution equipment; `outlet` identifies
individual PDU connections in the resource model. Pods illustrate a future
extension and do not currently have their own resource type. Solar, wind,
hypervisor and heat resources are not shown.

```kroki {_type=plantuml}
@startuml

	top to bottom direction

	skinparam nodesep 90
	skinparam ranksep 120

	circle GridA #Red
	circle GridB #Red
	circle GeneratorA #Red
	circle GeneratorB #Red
	circle BatteryA #Red
	circle BatteryB #Red


	circle PowerFeedA #Red
	circle PowerFeedB #Red


	circle PDU1A #Blue
	circle PDU1B #Blue
	circle PDU2A #Blue
	circle PDU2B #Blue
	circle PDU3A #Blue
	circle PDU3B #Blue


	GridA -right[hidden]-> GeneratorA
	GeneratorA -right[hidden]-> BatteryA
	BatteryA -right[hidden]-> GridB
	GridB -right[hidden]-> GeneratorB
	GeneratorB -right[hidden]-> BatteryB


	PowerFeedA -right[hidden]-> PowerFeedB


	PDU1A -right[hidden]-> PDU1B
	PDU1B -right[hidden]-> PDU2A
	PDU2A -right[hidden]-> PDU2B
	PDU2B -right[hidden]-> PDU3A
	PDU3A -right[hidden]-> PDU3B


	GridA -down-> PowerFeedA
	GridB -down-> PowerFeedB
	BatteryA -down-> PowerFeedA
	BatteryB -down-> PowerFeedB
	GeneratorA -down-> PowerFeedA
	GeneratorB -down-> PowerFeedB


	PowerFeedA -down-> PDU1A
	PowerFeedB -down-> PDU1B
	PowerFeedA -down-> PDU2A
	PowerFeedB -down-> PDU2B
	PowerFeedA -down-> PDU3A
	PowerFeedB -down-> PDU3B


	circle Baremetal1 #Blue
	circle Baremetal2 #Green
	circle Baremetal3 #Blue
	circle Baremetal4 #Blue
	circle Baremetal5 #Blue
	circle Baremetal6 #Green
	circle Baremetal7 #Blue
	circle Baremetal8 #Green
	circle Baremetal9 #Green
	circle Baremetal10 #Green
	circle Baremetal11 #Green
	circle Baremetal12 #Green


	circle VM1 #Green
	circle VM2 #Green
	circle VM3 #Green
	circle VM4 #Blue
	circle VM5 #Blue
	circle VM6 #Blue
	circle VM7 #Green

	circle Kubernetes1 #Green
	circle Kubernetes2 #Blue

	circle Pod1 #Green
	circle Pod2 #Green
	circle Pod3 #Green

	Baremetal1 -right[hidden]-> Baremetal2
	Baremetal2 -right[hidden]-> Baremetal3
	Baremetal3 -right[hidden]-> Baremetal4
	Baremetal4 -right[hidden]-> Baremetal5
	Baremetal5 -right[hidden]-> Baremetal6
	Baremetal6 -right[hidden]-> Baremetal7
	Baremetal7 -right[hidden]-> Baremetal8
	Baremetal8 -right[hidden]-> Baremetal9
	Baremetal9 -right[hidden]-> Baremetal10
	Baremetal10 -right[hidden]-> Baremetal11
	Baremetal11 -right[hidden]-> Baremetal12


	VM1 -right[hidden]-> VM2
	VM2 -right[hidden]-> VM3
	VM3 -right[hidden]-> VM4
	VM4 -right[hidden]-> VM5
	VM5 -right[hidden]-> VM6
	VM6 -right[hidden]-> VM7


	Kubernetes1 -right[hidden]-> Kubernetes2


	Pod1 -right[hidden]-> Pod2
	Pod2 -right[hidden]-> Pod3


	PDU1A -down-> Baremetal1
	PDU1B -down-> Baremetal1
	PDU1A -down-> Baremetal2
	PDU1B -down-> Baremetal2
	PDU1A -down-> Baremetal3
	PDU1B -down-> Baremetal3
	PDU1A -down-> Baremetal4
	PDU1B -down-> Baremetal4
	PDU2A -down-> Baremetal5
	PDU2B -down-> Baremetal5
	PDU2A -down-> Baremetal6
	PDU2B -down-> Baremetal6
	PDU2A -down-> Baremetal7
	PDU2B -down-> Baremetal7
	PDU2A -down-> Baremetal8
	PDU2B -down-> Baremetal8
	PDU3A -down-> Baremetal9
	PDU3B -down-> Baremetal9
	PDU3A -down-> Baremetal10
	PDU3B -down-> Baremetal10
	PDU3A -down-> Baremetal11
	PDU3B -down-> Baremetal11
	PDU3A -down-> Baremetal12
	PDU3B -down-> Baremetal12



	Baremetal1 -down-> VM1
	Baremetal1 -down-> VM2
	Baremetal1 -down-> VM3
	Baremetal3 -down-> VM4
	Baremetal3 -down-> VM5
	Baremetal5 -down-> VM6
	Baremetal5 -down-> VM7


	VM4 -down-> Kubernetes1
	VM5 -down-> Kubernetes1
	VM6 -down-> Kubernetes1

	Baremetal4 -down-> Kubernetes2
	Baremetal7 -down-> Kubernetes2


	Kubernetes2 -down-> Pod1
	Kubernetes2 -down-> Pod2
	Kubernetes2 -down-> Pod3

@enduml
```

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
