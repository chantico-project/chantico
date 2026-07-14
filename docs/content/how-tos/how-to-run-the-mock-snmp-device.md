
---
title: "How to run the mock snmp device"
menus:
  main:
    parent: howto
    weight: 20
---

## The SNMP mock

The SNMP mock is an UDP server mocking a device using SNMP with a mock MIB file 
(`./dev/mibs/TNO-PDU-MIB.txt`). It provides random energy values for the 
following metrics: `tnoPduEnergyValue` and `tnoPduPowerValue`. This file details 
how to set up the mock device, and how to subsequently run a demo with it 
including both the `PhysicalMeasurement` and `MeasurementDevice` custom 
resources.

### Requirements

Ensure you have followed the [installation](how-to-install-chantico.md) 
guidance or the instructions in [How to set up the local development 
environment](how-to-setup-the-local-development-environment.md) to set up 
a production-like or local development environment. After this:

- The cluster is running.
- The controller is running (locally via `make run` or in-cluster).

## Manual installation

Note that the SNMP mock is part of the local development environment, so you do 
not need to follow the manual installation steps here if you are fine with the 
default latest version of the mock image. Only in cases where you need to update 
the mock image during development, or when deploying the mock into a separate 
cluster, follow the manual installation steps below. Otherwise, skip to the 
section on running the demo with the mock SNMP device.

### Load the snmp-mock image into the kind environment

To obtain the latest SNMP mock image, pull it from the GitHub Container Registry 
and load it into the kind cluster:

```bash
export CI_REGISTRY="ghcr.io/chantico-project/images"
export SNMP_MOCK_TAG="${SNMP_MOCK_TAG:-latest}"
export SNMP_MOCK_IMAGE="$CI_REGISTRY/chantico-snmp-mock:$SNMP_MOCK_TAG"
docker pull "$SNMP_MOCK_IMAGE"
docker tag "$SNMP_MOCK_IMAGE" chantico-snmp-mock:latest
kind load docker-image chantico-snmp-mock:latest --name kind
```

Alternatively, you can build the image locally and load it into the kind cluster:

```bash
docker build -t chantico-snmp-mock:latest -f Dockerfile.snmp-mock .
kind load docker-image chantico-snmp-mock:latest --name kind
```

### Apply the mock to Kubernetes

```bash
kubectl apply -f dev/k8s/snmp-mock-deployment.yaml
kubectl apply -f dev/k8s/snmp-mock-service.yaml
kubectl apply -f config/samples/chantico_v1alpha1_physicalmeasurement_mock.yaml
```

## Running the demo with the mock SNMP device

### Querying the chantico-snmp-mock running in the development setup

If the development kind cluster is running the `chantico-snmp-mock` service, there is a Node Port that is visible on port `31161`.

It can be queried as follow:

```bash
snmpget -v2c -c public -M +./dev -m +TNO-PDU-MIB localhost:31161 tnoPduEnergyValue
```

### Chantico workflow with the snmp-mock as snmp device (full demo)

For an overview of the workflow that is run in the background in this demo please see the below image.

![](../puml/PhysicalMeasurement-and-MeasurementDevice-sequence.png)

This section demonstrates a full flow: MIB upload → `MeasurementDevice` → `PhysicalMeasurement` → Prometheus targets.

1. Upload the MIB file `./dev/mibs/TNO-PDU-MIB.txt` to the cluster:
    ```bash
    make copy-mock-mib
    ```
1. Create a `MeasurementDevice` for the mock MIB:
    ```bash
    kubectl apply -f ./config/samples/chantico_v1alpha1_measurementdevice_mock.yaml
    ```
1. Wait for the SNMP generator job:
    ```bash
    kubectl get jobs -n chantico | grep update-snmp
1. Create a `PhysicalMeasurement` pointing at the mock target:
    ```bash
    kubectl apply -f ./config/samples/chantico_v1alpha1_physicalmeasurement_mock.yaml
    ```
1. Port-forward Prometheus (if not already done by the local development 
   environment) and verify targets:
    ```bash
    kubectl port-forward -n chantico deployment/chantico-prometheus 19090:9090
    ```
    Open http://localhost:19090/targets and verify the target is `UP`.

### Adding another mock snmp device

The same mock SNMP image can be deployed multiple times to simulate additional devices. Each additional device needs its own Deployment, Service, `MeasurementDevice`, and `PhysicalMeasurement`. Example manifests for a second mock device are provided in the repository.

1. Deploy the second mock SNMP agent:
    ```bash
    kubectl apply -f dev/k8s/snmp-mock-2-deployment.yaml
    kubectl apply -f dev/k8s/snmp-mock-2-service.yaml
    ```
1. Create a second `MeasurementDevice` (can reuse the same MIB/walks, or use 
   different ones):
    ```bash
    kubectl apply -f ./config/samples/chantico_v1alpha1_measurementdevice_mock2.yaml
    ```
1. Wait for the SNMP generator job for the new device:
    ```bash
    kubectl get jobs -n chantico | grep update-snmp
    ```
1. Create a second `PhysicalMeasurement` pointing at the new mock target:
    ```bash
    kubectl apply -f ./config/samples/chantico_v1alpha1_physicalmeasurement_mock2.yaml
    ```
1. Verify both targets appear in Prometheus, port-forwarding if not already done 
   by the local development environment:
    ```bash
    kubectl port-forward -n chantico deployment/chantico-prometheus 19090:9090
    ```

    Open http://localhost:19090/targets — both the original and the new target 
    should show as `UP`.

The second mock is exposed on NodePort `31162`, so you can also query it directly:

```bash
snmpget -v2c -c public -M +./dev -m +TNO-PDU-MIB localhost:31162 tnoPduEnergyValue
```

> **Tip:** To add more devices beyond the second, copy the mock-2 manifests, update the names (e.g. `chantico-snmp-mock-3`), pick a free NodePort, and create corresponding `MeasurementDevice` / `PhysicalMeasurement` resources. Prometheus will automatically pick up new targets via `file_sd_configs` — no restart required.

### Adding baremetal resources and linking them to the mock SNMP device

To demonstrate the full workflow, you can also create a `DataCenterResource` representing a baremetal resource and link it to the mock SNMP device. This makes it possible to aggregate metrics from the SNMP devices together into the data center resource, store the metrics in VictoriaMetrics and visualize them in Grafana.

To do so, you can apply the following manifest:

```bash
kubectl apply -f ./config/samples/chantico_v1alpha1_datacenterresource.yaml
```

If all the resources are created correctly, you should see the following when querying the resources in the `chantico` namespace on your cluster:

```bash
$ kubectl get datacenterresource,measurementdevice,physicalmeasurement -n chantico
NAME                                                                           AGE
datacenterresource.chantico-project.github.io/datacenterresource-misd-gbm-01
datacenterresource.chantico-project.github.io/datacenterresource-pdu1
datacenterresource.chantico-project.github.io/datacenterresource-pdu2

NAME                                                 STATUS   REASON      TYPE             AGE
measurementdevice.chantico-project.github.io/tno     True     Succeeded   ExporterReload
measurementdevice.chantico-project.github.io/tno-2   True     Succeeded   ExporterReload

NAME                                                                          AGE
physicalmeasurement.chantico-project.github.io/physicalmeasurement-pdu1-out
physicalmeasurement.chantico-project.github.io/physicalmeasurement-pdu2-out
```

Additionally, you can verify in [Prometheus](http://localhost:19090) that the metrics are being scraped from the mock SNMP devices and aggregated into the `DataCenterResource` metrics. These will show up in the "Rule health" section as the following metrics:
- `datacenter:dataceneterresource_pdu1:energy_watts`
- `datacenter:dataceneterresource_pdu2:energy_watts`
- `datacenter:dataceneterresource_misd_gbm_01:energy_watts`

You can query these metrics from Prometheus for the recent data of the specific time series. You can also visualize these metrics in [Grafana](http://localhost:13000) by visiting the pre-configured "Chantico" dashboard.
