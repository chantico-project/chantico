---
title: "Milestone 1 (current)"
main:
  parent: roadmap
  weight: 20
---

## Milestone 1

### Introduction

In our scope and vision for the development roadmap of chantico, we aim to be 
able to monitor the power consumption of the bare metal servers in a lab-like 
data center environment, where we are able to monitor PDUs and servers via SNMP 
and IPMI interfaces. We combine measurements from multiple PDUs in case the bare 
metal server is plugged into multiple outlets.

If the server is also able to be monitored via IPMI, we can also combine the 
measurements from the PDUs and the bare metal server to have a more accurate 
estimation of the power consumption of the server. Any difference between these 
measurements are reported as a separate metric toward the data center operator 
to indicate what the power consumption of the non-customer facing components of 
the server is. This includes for example the onboard management controller (BMC) 
which might not be monitored by the BMC itself but does consume power.

We provide a simple UI, accessible via the browser, that shows the timeseries of the energy usage of the server. We currently assume that the energy usage of the outlets of the PDU is equal to the total energy consumption of the server. 

### Use case

In this use case of the first milestone, the User is a data center operator who wants to monitor energy consumption of bare metal servers that are provided as a service for workloads.

1. User installs Chantico operator using the Helm charts.
2. User has MIBs stored in Git. The location of these MIBs are accessible by Chantico.
3. User purely interacts with Chantico via Kubernetes manifests (this may be through an automated process such as a workflow orchestrator, or manually). There will be no other/HTTP API support.
4. User can create and delete Kubernetes manifests. Updates of manifests by the user are outside the scope of the use case.
5. User can define data center resources to generate a graph that describes an aggregation of measurements. For this milestone, only links for PDU/outlet to Server can be defined. For, Server either IPMI or SNMP will be supported. These combined measurements enable monitoring of BareMetal services. For now, no higher-level services like virtual machines, etc.
5. User can see the resulting timeseries for the BareMetal service in a simple UI. The UI application will directly talk with Prometheus. For now, there won't be an exposed API for other applications to integrate with.


### Description details

- Be able to use MIBs synced from a GitOps repo (not event-driven) to provide to 
  chantico
- Dynamic PDU outlet to server timeseries name mapping
- Basic aggregation and distribution on PDU/baremetal levels: to support when 
  a server does not have (reliable) SNMP for PDU inputs or when the measurements 
  are different due to IPMI/server/overhead.
- Create and delete custom resources. Stretch goal: Properly handle modification 
  of all our resources.
- Proper ownership of controller to custom resource manifests, jobs, deployments 
  and other Kubernetes resources created by the controller. Stretch goal: Handle 
  ownership of the Helm chart (for Prometheus, etc.) deployed along with the 
  controller.
- Output for dashboards (Chantico Charts, embedded Grafana, or simple metrics in 
  graph visualization webapp) running as a frontend service in cluster (part of 
  deployment of operator).

Keep in mind during that we extend this use case later with VM monitoring, 
timeseries aggregation/overhead distribution/mapping. Research/exploration to 
extend this is part of this milestone

### Limitations

#### Out of scope

- We only provide interactions via Kubernetes manifests. We will not support an API.
- Migrations of workloads between servers inside the same data center or between 
  federated data centers. We do not track updates of server/service mappings.

#### Exploration topics

- Our focus is currently on collecting energy consumption metrics from SNMP 
  endpoints of PDUs and bare metals, while our vision is also on collecting 
  usage metrics of VMs from hypervisors and for pods in clusters.
- The current resource definitions of data center resources, physical 
  measurements and measurement devices have no simple means of indicating which 
  outlet of a PDU is involved (leads to duplicate scrape endpoint physical 
  measurement resources). We plan to support some kind of mapping from connected 
  PDU to bare metal server.
- The aforementioned mapping should be generic enough to allow reuse in 
  mappings from timeseries related to VMs, to allow translating internal unique 
  IDs from hypervisors to service IDs provided by the orchestrator. Preferably, 
  this mapping should be easily translated into Prometheus configuration to 
  perform rule-based discovery.
- Our first use case focuses on providing (raw/combined) time series streams of 
  energy consumption of bare metals based on measurements on PDU and bare 
  metals. When we are able to monitor both connected PDUs and the involved bare 
  metal, there may be differences in the energy measurements. These can be 
  assigned to measurement errors, exclusion of onboard BMC (IPMI/RAC "mini 
  servers" hosted in the same server unit) or other interfaces. A fair algorithm 
  that determines how to allocate the overhead to the data center operator 
  and/or server consumer is to be designed.

### Deployment

For our development cycle, we primarily test on local development environments. 
As part of the first milestone, we will also deploy the Chantico stack to 
a cluster hosted in [HESI lab](https://fasttrack.tno.nl/activiteiten/hesi-lab/), 
a lab-like system integration environment. This includes monitoring of PDUs and 
bare metal servers in this lab cluster. Deployment takes place with the Helm 
charts provided in the repository and with pre-built Docker images.

### Diagrams

We are currently working on diagrams. These diagrams will be published here.
