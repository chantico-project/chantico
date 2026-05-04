---
title: "How to register an SNMP device type"
menus:
  main:
    parent: howto
    weight: 30
---

In the current setting, a type of device using SNMP is configured by uploading MIBs and defining a `MeasurementDevice` custom resource.
The operator generates SNMP module config (`snmp.yml`) and triggers a reload of `chantico-snmp`.
In our First use-case (see `goal.md`) this corresponds to the `registerPDU` phase.

1. Upload the MIBS:
  1. Port-forward the filebrowser
  ```sh
  kubectl port-forward -n chantico deployment/chantico-filebrowser 18888:80
  ```
  1. Login with (user: admin, password: admin)
  1. Upload your MIBS files in `snmp/mibs`
1. Create the MeasurementDevice matching the required type of MeasurementDevice
  1. Create a `measurement_device.yaml` file

  ```yaml
  apiVersion: chantico.ci.tno.nl/v1alpha1
  kind: MeasurementDevice
  metadata:
    labels:
      app.kubernetes.io/name: chantico
      app.kubernetes.io/managed-by: kustomize
    name: example-measurement-device
    namespace: chantico
  spec:
    auth:
      community: public
      version: 2
    walks:
      - sdbDevInKWhTotal
  ```
  1. Apply the yaml file
  ```sh
  kubectl apply -f measurement_device.yaml
  ```
1. Verify the new device setting
  1. Wait for the SNMP generator job to complete
  ```sh
  kubectl get jobs -n chantico | grep update-snmp
  ```
  1. The generated config is stored on the shared volume at `snmp/yml/snmp.yml`.
  1. Port-forward the SNMP exporter
  ```sh
  kubectl port-forward -n chantico deployment/chantico-snmp 9116:9116
  ```
  1. Check that the config (http://localhost:9116/config) include the registered device as a module 
