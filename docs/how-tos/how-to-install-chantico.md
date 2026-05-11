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
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico # Latest version
```

- The command above installs the latest version of Chantico. See available [chart versions](https://github.com/chantico-project/chantico/pkgs/container/charts%2Fchantico). Also check out the [releases](https://github.com/chantico-project/chantico/releases) or the [changelog](/technical/changelog.md) on the documentation website for the list of changes throughout the version history.

- Inspect the [values.yaml](https://github.com/chantico-project/chantico/blob/main/config/deployment/values.yaml) file to see what parameters can be provided. For example, excluding the Chantico controller from the installation can be done with `--set controller.include=false`.

## Upgrading using OCI Registry

To upgrade an existing chantico deployment to a new version, run `helm upgrade` with the new version provided.

```bash
helm upgrade chantico oci://ghcr.io/chantico-project/charts/chantico --version <version> -n chantico
```

## Getting started with the deployed Chantico

After Chantico is successfully deployed on your cluster, you can start making use of it for measuring your datacenter hardware of interest. Currently this can only be done with manual configuration, until a more automated approach has been implemented. Chantico inherently configures SNMP walks for endpoints by means of `MIB` and `.yaml` files. The steps of configuring this typically follows the following how-to guides:

1. [How to register an SNMP device type](how-to-register-an-snmp-device-type.md) - Upload the MIB files to use and make `.yaml` files for measurement devices. Also see the example at `config/samples/chantico_v1alpha1_measurementdevice.yaml`.
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
