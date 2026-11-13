---
title: "How to install Chantico"
menus:
  main:
    parent: howto
    weight: 20
---

## Installation of Chantico from OCI Registry

The easiest option to install Chantico into your k8s cluster is by using the helm package from the OCI Registry.

```bash
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico --create-namespace # Latest version
```

- The command above installs the latest version of Chantico. See available [chart versions](https://github.com/chantico-project/chantico/pkgs/container/charts%2Fchantico). Also check out the [releases](https://github.com/chantico-project/chantico/releases) or the [changelog](/technical/changelog.md) on the documentation website for the list of changes throughout the version history.

- Inspect the [values.yaml](https://github.com/chantico-project/chantico/blob/main/chart/values.yaml) file to see what parameters can be provided. For example, excluding the Chantico controller from the installation can be done with `--set controller.include=false`.

## Upgrading using OCI Registry

To upgrade an existing chantico deployment to a new version, run `helm upgrade` with the new version provided.

```bash
helm upgrade chantico oci://ghcr.io/chantico-project/charts/chantico --version <version> -n chantico
```

## Using an alternative storage class

The Chantico chart creates two persistent volumes:

- `pvc`: claim with data shared by the controller, SNMP generator jobs, filebrowser and Prometheus.
- `victoriaMetrics.pvc`: settings of the claim template of the VictoriaMetrics StatefulSet.

By default, both claims use the default storage class of the cluster. Override `storageClassName` when installing or upgrading Chantico. Each block also accepts `storage`, `accessModes` and `volumeName`.

```bash
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico --create-namespace \
  --set pvc.storageClassName="STORAGE_CLASS_NAME"
```

The shared claim name is configured with `persistentVolumeClaimName`. To use an existing claim, set its name and disable creation with `pvc.include=false`. Disabling creation does not disable the volume mount.

### VictoriaMetrics storage

VictoriaMetrics runs as a StatefulSet that creates its own claim, `chantico-volume-mount-chantico-victoriametrics-0`, from a claim template. Set a different storage class or size with `victoriaMetrics.pvc`.

```bash
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico --create-namespace \
  --set victoriaMetrics.pvc.storageClassName="STORAGE_CLASS_NAME" \
  --set victoriaMetrics.pvc.storage="10Gi"
```

To bind the claim to a specific existing volume, set `victoriaMetrics.pvc.volumeName`. The claim is not removed when the StatefulSet or release is deleted, so VictoriaMetrics data is kept. VictoriaMetrics stores its data in the `victoria-metrics/` directory of its claim.

You can list the storage classes available on your cluster with kubectl.

```bash
kubectl get storageclass
```

### Multi-node clusters

With the default `ReadWriteOnce` access mode, the shared volume can only be attached to one node. The chart and the controller therefore schedule all pods that use it, including SNMP generator jobs and Prometheus, on the same node. Required pod affinity selects pods using the same claim in the same namespace and uses `kubernetes.io/hostname` as the topology key.

VictoriaMetrics has its own claim and no affinity to the shared volume, so it can run on a different node.

Use a CSI storage class for multi-node deployments. The chart's optional `hostPath` PV remains intended for single-node development.

## Watching resources in different namespaces

By default, the Chantico controller manages resources in the namespace it is deployed in.
If you want it to watch resources in a different namespace, you can override the `controller.watchNamespace` value when installing or upgrading Chantico. You can set this value to a single namespace or to an asterisk to give access to the managed resources in all namespaces.

```bash
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico --create-namespace --set controller.watchNamespace="*"
```

## Getting started with the deployed Chantico

After Chantico is successfully deployed on your cluster, you can start making use of it for measuring your datacenter hardware of interest. Currently this can only be done with manual configuration, until a more automated approach has been implemented. Chantico inherently configures SNMP walks for endpoints by means of `MIB` and `.yaml` files. The steps of configuring this typically follows the following how-to guides:

1. [How to register an SNMP device type](how-to-register-an-snmp-device-type.md) - Upload the MIB files to use and make `.yaml` files for measurement devices. Also see the example at `config/samples/chantico_v1alpha1_measurementdevice_mock.yaml`.
1. [How to register a physical SNMP device](how-to-register-a-physical-snmp-device.md) - Define IP address(es) of interest in physical measurement `.yaml` file. Example at `config/samples/chantico_v1alpha1_physicalmeasurement.yaml`.
1. With the MIB files, measurement devices and physical measurements in place, the targets should be accessible and scrapeable in Prometheus. Perform port forwarding on the Prometheus deployment to validate the result of this setup. If done successful one should see a timeseries of the requested value(s).
1. [How to register data center resources](how-to-register-data-center-resources.md) When desired, encapsulate data center structure using data center resources.

### Uninstall Chantico on K8s cluster

1. Remove Chantico together with dependencies like CRDs:

```bash
helm uninstall chantico -n chantico
```

2. Optionally, delete the namespace:

```bash
kubectl delete namespace chantico
```
