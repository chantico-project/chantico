---
title: "Milestone 2 (current)"
main:
  parent: roadmap
  weight: 20
---

## Milestone 2

### Introduction

After our first milestone, which focused on monitoring the power consumption of 
power distribution units (PDUs) and bare metal servers, we want to extend the 
functionality to include monitoring of virtual machines (VMs) running on these 
servers, while also keeping in mind to keep code clean and reusable. This 
includes making it easier to use and configure chantico. We extend upon the 
recording rules to aggregate metrics from one layer of the data center resource 
graph to the next. We make this process work regardless of what type of 
resources are involved and where the metrics come from. As such, we also want to 
track any differences between measured energy values, including overhead of 
server components not provided to the service on a bare metal, and similarly for 
overhead of the hypervisor.

This milestone will focus on exploring and implementing features that allow us 
to monitor VMs, as well as improving the architecture and integration 
capabilities of chantico.

We demonstrate the integrated and working functionality via a simple UI built as 
a Grafana dashboard, which is deployed as part of the operator deployment and we 
will augment it with new metrics. Other web apps and APIs are potential stretch 
objectives.

### Context of goals

- We want to further configure chantico for testing in the HESI lab, our first 
  testing location.
- We want to deploy an initial version of the chantico package to Staging lab, 
  our second demo location.
- We have moved everything to GitHub (pipeline, issues and milestone planning, 
  documentation website deployment, container registry, etc.), but we want to 
  further improve our setup of the GitHub Actions pipeline and the checks that 
  are reported in Pull Requests to help guide new external contributors in their 
  efforts to bring their additions to chantico, when it is in line with our 
  milestone goals.

### Exploration topics and features under consideration

- We want to include VM monitoring. Specific hypervisors to support are not yet 
  decided, but our initial aim is to include OpenNebula and/or Proxmox.
- We want to provide more integration capabilities to allow users, orchestration 
  frameworks and other external tools to retrieve timeseries information. This 
  might also include an API reference to create/read/update/delete the resource 
  information, although it is possible we will keep this part Kubernetes-native.

#### VM monitoring

We want to explore the possibility of monitoring VMs running on the bare metal 
servers. We want to use existing tooling to monitor VMs, such as the 
hypervisor's own monitoring capabilities. The list of hypervisors we consider is 
mentioned in the exploration topics above. We also consider tools like 
[Kepler](https://sustainable-computing.io/), 
[Alumet](https://alumet-dev.github.io/user-book/) and 
[Scaphandre](https://hubblo-org.github.io/scaphandre-documentation/) and support 
this as a pluggable component in chantico. We explore the possibility to ingest 
metrics from these tools in a standardized way, at least such that Prometheus is 
able to scrape the metrics from these tools. We want to support efforts like 
[OpenTelemetry](https://opentelemetry.io/) to standardize the metric collection 
framework.

### Out of scope

- We do not plan to include any support for monitoring of Kubernetes clusters 
  and containers in this milestone.
- Migrations of workloads between servers and/or hypervisors, inside the same 
  data center or between federated data centers, are not envisioned to be 
  supported in this milestone. We do not track updates of server, VM and service 
  mappings, aside from deletions.

### Deployment

For our development cycle, we primarily test on local development environments. 
As part of the first milestone, we will also deploy the Chantico stack to 
a cluster hosted in [HESI lab](https://fasttrack.tno.nl/activiteiten/hesi-lab/), 
a lab-like system integration environment. This includes monitoring of PDUs and 
bare metal servers in this lab cluster. Deployment takes place with the Helm 
charts provided in the repository and with pre-built Docker images.

### Architecture improvements

Parallel (and sometimes prior) to the work on the features of this milestone, we 
want to improve the architecture of chantico. This includes:

- Upgrading controllers to use step-based function logic instead of internal 
  state machine. This means that the controller writes separate status events 
  into the resource status field, but does not use its own resource state to 
  determine where it left off during a later reconciliation. Instead, the actual 
  state is checked by looking for existence of files, changes to configurations 
  of services, etc. before deciding which step to execute.
- Onboarding of Sigrid to track code quality, maintainability and open source 
  health (license and security issues). We further configure Sigrid to track 
  components to be able to view the changes to code quality over time. The 
  improvement of quality is a continuous process, and we do not intend to 
  specifically target certain components or metrics to score better, but instead 
  use it as a guide and hope to obtain some feedback from the tool and team to 
  also help with reviewing of Pull Requests.
- Exploration of different architecture of the core and associated deployments, 
  to make it easier to extend with new functionality, such as a carbon emission 
  reporting component, and to allow the VM monitoring to be developed in 
  a pluggable manner. This makes it easier to extend support to different 
  hypervisors while keeping the core of chantico agnostic. Similar approach will 
  be taken for common functionality like reading/writing files by the operator 
  to allow later extension with different storage backends, such as object 
  storage or S3 buckets, and simplifying code using existing modules to reduce 
  code duplication between controllers.
- We want to make modules in the controller that are used by other components 
  and themselves do not use anything aside from building on top of existing 
  external libraries, to limit calls between different packages. Also to avoid 
  global configuration being shared between places, and instead use dependency 
  injection to pass configuration where it is needed, also to make testing more 
  simplified and later support easier end-to-end integration testing using 
  different environments.
