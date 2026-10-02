---
title: "How to run the VM Attribution mock"
menus:
  main:
    parent: howto
    weight: 20
---

## The VM Attribution mock

The VM Attribution mock simulates a hypervisor Prometheus exporter that reports which
which fraction of a baremetal host's usage is attributed to each of the virtual
machines running on it. It serves a single metric on the `/metrics` endpoint:

```text
vm_attribution_coefficient{name="prometheus-mock-bm-1",resource_consumer_id="/qemu.slice/120.scope"} 0.1
```

The coefficients always sum to less than one, leaving part of the host usage
unattributed to any VM (ie. system overhead).

### Simulated values

Like the [SNMP mock](how-to-run-the-mock-snmp-device.md), the values are not 
random. The attribution coefficient drifts by at most a fixed variance
per scrape and stays between a lower and an upper bound. The reported
coefficient is that weight relative to the total.

The mock is configured with environment variables on its container (see
`dev/k8s/vm-attribution-mock-deployment.yaml`):

| Variable | Default | Description |
| --- | --- | --- |
| `MOCK_PROMETHEUS_HOST_NAME` | `prometheus-mock-bm-1` | Host reported in the `name` label. |
| `MOCK_PROMETHEUS_VM_IDS` | `120,121` | Comma separated VM IDs to report. |
| `MOCK_PROMETHEUS_PORT` | `9090` | Port the HTTP server listens on. |
| `MOCK_PROMETHEUS_WEIGHT_MIN` | `0.2` | Lower bound of a VM weight. |
| `MOCK_PROMETHEUS_WEIGHT_MAX` | `1.0` | Upper bound of a VM weight. |
| `MOCK_PROMETHEUS_WEIGHT_START` | `0.6` | Weight used on the first scrape. |
| `MOCK_PROMETHEUS_WEIGHT_VARIANCE` | `0.02` | Maximum change per scrape, in either direction. |

### Manual installation

The mock is part of the local development environment. After deploying Chantico (which is deployed by
`make cluster-configure`), you can then deploy the mocks with:

```bash
make docker-pull-mocks
make cluster-mocks
```

This deploys both the mock deployment and the service for both the VM Attribution mock and the SNMP mock (see [how to run the SNMP mock](how-to-run-the-mock-snmp-device.md)).

### Querying the mock

The service is exposed on NodePort `30909`, which the development kind cluster
maps to host port `19091`:

```bash
curl http://localhost:19091/metrics
```

Repeated scrapes should show the coefficients changing only slightly.
