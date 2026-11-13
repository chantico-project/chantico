---
title: "How to attribute energy to a virtual machine"
menus:
  main:
    parent: howto
    weight: 40
---

This guide describes one way to attribute a bare-metal server's measured energy
to the virtual machines (VMs) running on it. Alumet collects host and
cgroup-related measurements and exposes them in Prometheus format. Chantico
scrapes those metrics and uses a coefficient on each VM's relationship to its
host to calculate the VM's share.

This setup assumes that Chantico and Prometheus are already running, and that
the bare-metal server and its upstream energy measurement (for example, a PDU)
are represented in Chantico. For information about defining resources and
relationships, see the
[how to register data center resources](how-to-register-data-center-resources.md).

## Topology overview

The diagram shows an example hypervisor hosting two VMs. Alumet runs directly
on the hypervisor. The Bare Metal server receives power from two PDU sockets.

```kroki {_type=d2 _output=embed}
vars: {
  d2-config: {
    layout-engine: elk
  }
}

direction: right

socket1: "PDU Socket 1"
socket2: "PDU Socket 2"

hypervisor: {
  label: "Hypervisor"
  style: {
    fill: "#FFF3E0"
    stroke: "#E69B00"
  }

  alumet: "Alumet Agent" {
    style: {
      fill: "#FFE082"
      stroke: "#8D6E00"
    }
  }
}

vm1: "Virtual Machine 1"
vm2: "Virtual Machine 2"

socket1 -> hypervisor
socket2 -> hypervisor
hypervisor -> vm1
hypervisor -> vm2
```

In this example we would have 3 `PhysicalMeasurement`s, and 5 `DataCenterResource`s. 

## 1. Install Alumet on the hypervisor

Install the Alumet agent on the hypervisor that runs the VMs. Follow the
[Alumet installation guide](https://alumet-dev.github.io/user-book/start/install.html)
for your operating system and installation method.

## 2. Configure Alumet

The example enables Alumet's Prometheus exporter, cgroups, performance events,
and procfs plugins for kernel, memory, network, and process metrics. **Enable
the exporter** so Chantico's Prometheus instance can scrape its endpoint. The
endpoint must be reachable from the cluster.

Configure Alumet so it can observe the host's cgroups and collect the metrics
needed for attribution. If you run Alumet in a container, ensure it has the
required access to the host's cgroups and performance events; an isolated
container will not observe the host's VM processes.

```toml
[plugins.prometheus-exporter]
host = "0.0.0.0"
prefix = ""
suffix = "_alumet"
port = 9091
add_attributes_to_labels = true

[plugins.cgroups]
poll_interval = "1s"
disable_sources = false

[plugins.perf]
poll_interval = "1s"
flush_interval = "5s"
events = [
    "REF_CPU_CYCLES",
    "CACHE_MISSES",
    "BRANCH_MISSES",
    "LL_READ_MISS",
]
add_source_in_pause_state = false

[plugins.procfs.kernel]
enabled = true
poll_interval = "5s"

[plugins.procfs.memory]
enabled = true
poll_interval = "5s"
metrics = [
    "MemTotal",
    "MemFree",
    "MemAvailable",
    "Cached",
    "SwapCached",
    "Active",
    "Inactive",
    "Mapped",
]

[plugins.procfs.network]
enabled = true
poll_interval = "5s"

[plugins.procfs.processes]
enabled = true
refresh_interval = "2s"
strategy = "watcher"

[[plugins.procfs.processes.groups]]
exe_regex = ""
poll_interval = "2s"
flush_interval = "4s"
memory_mode = "quick"

[plugins.procfs.processes.events]
poll_interval = "1s"
flush_interval = "4s"
memory_mode = "quick"
```

Start Alumet using the method for your installation, then check that its
Prometheus exporter is available on port `9091` (or the port you configured).
The exporter must be reachable from Chantico's Prometheus pod. If the
hypervisor is not directly reachable from the cluster, arrange network access
before adding the scrape target.

## 3. Add the Alumet exporter as a measurement target

Create a `PhysicalMeasurement` that points to the hypervisor's Alumet exporter.
Set `spec.ip` to an address and port reachable from Prometheus:

```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: PhysicalMeasurement
metadata:
  name: physicalmeasurement-hypervisor-01
  namespace: chantico
spec:
  type: prometheus-exporter
  ip: 192.0.2.10:9091
```

Apply the manifest:

```sh
kubectl apply -f physicalmeasurement-hypervisor-01.yaml
```

Chantico adds this target to Prometheus's exporter scrape configuration. Check
that it is ready and that Prometheus can scrape it:

```sh
kubectl describe -n chantico physicalmeasurement physicalmeasurement-hypervisor-01
```

In Prometheus, open **Status > Targets** and confirm the target is `UP`. You
can also query a metric shown by the Alumet endpoint to confirm its series and
labels are available.

## 4. Add VM resources and attribution coefficients

Create a `DataCenterResource` for each VM and set the bare-metal resource as
its parent. Use a coefficient query that selects the series for that VM and
returns its share of the host's energy.

The repository includes a basic Proxmox 9 Alumet template in
[proxmox_alumet_vm_attribution_basic.yaml](../../../config/samples/attribution-templates/proxmox_alumet_vm_attribution_basic.yaml).
It uses `cpu_percent_alumet_ratio` and expects `resourceName` to match the
hypervisor's `PhysicalMeasurement` name and `vmid` to match the VM's ID in Proxmox. Apply
it from the repository root:

```sh
kubectl apply -n chantico -f config/samples/attribution-templates/proxmox_alumet_vm_attribution_basic.yaml
```

If you use a different hypervisor, metric, or label scheme, create your own
ConfigMap template with a query that returns a coefficient between 0 and 1.
The coefficient represents the proportion of the bare-metal server's energy
attributable to the VM. Change the ConfigMap name in the VM resource to match
your own:

```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-hypervisor-01-vm-120
  namespace: chantico
spec:
  type: vm
  parents:
    - name: datacenterresource-hypervisor-01
      coefficientFrom:
        valueFrom:
          configMapKeyRef:
            name: proxmox9-alumet-energy-attribution-coefficient-template
            key: template
        templateParameters:
          - name: resourceName
            value: physicalmeasurement-hypervisor-01
          - name: vmid
            value: "120"
```

Replace `datacenterresource-hypervisor-01` with the name of the existing
bare-metal `DataCenterResource`, and set the VM identifier to match the
hypervisor's cgroup. Repeat the VM resource with the appropriate cgroup for
each VM. The ConfigMap and resources must be in the same namespace.

Apply the manifest and check the resource status:

```sh
kubectl apply -f vm-attribution.yaml
kubectl describe -n chantico datacenterresource datacenterresource-hypervisor-01-vm-120
```

Confirm the coefficient expression returns the intended series in Prometheus.
If it returns no series, check the metric name, cgroup identifier, labels, and
that the Alumet exporter is being scraped. A coefficient is evaluated for the
VM's edge to its parent, so ensure the parent resource is the host whose energy
is being apportioned.
