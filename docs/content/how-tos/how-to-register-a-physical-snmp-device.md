---
title: "How to register a physical snmp device"
menus:
  main:
    parent: howto
    weight: 30
---

`PhysicalMeasurement` does not parse MIBs. It links a concrete device IP to an existing `MeasurementDevice` (SNMP module/auth definition) and creates a Prometheus Operator `ScrapeConfig` for it. Prometheus Operator applies the scrape config to Prometheus.
In our First use-case (see `goal.md`) this corresponds to the `createPDU1` and `createPDU2` phases.

1. Ensure a matching `MeasurementDevice` exists (see `how-to-register-an-snmp-device-type.md`).
1. Create the PhysicalMeasurement matching the required type of PhysicalMeasurement
  1. Create a `physical_measurement.yaml` file

  ```yaml
  apiVersion: chantico-project.github.io/v1alpha1
  kind: PhysicalMeasurement
  metadata:
    labels:
      app.kubernetes.io/name: chantico
      app.kubernetes.io/managed-by: kustomize
    name: physicalmeasurement-pdu1-out
    namespace: chantico
  spec:
    ip: 10.5.1.1
    measurementDevice:  schleifenbauer-out # This has to be a currently valid MeasurementDevice name
  ```
  1. Apply the yaml file
  ```sh
  kubectl apply -f physical_measurement.yaml
  ```
1. Verify the new device setting
  1. Check that a `ScrapeConfig` was created with the same name as the `PhysicalMeasurement`
  ```sh
  kubectl get scrapeconfig -n chantico physicalmeasurement-pdu1-out
  ```
  1. Port-forward Prometheus
  ```sh
  kubectl port-forward -n chantico svc/chantico-prometheus 9090:9090
  ```
  1. Check the targets at http://localhost:9090/targets. It can take a few seconds for Prometheus Operator to apply the new scrape config.
  1. If you are using the local development setup with kind, Prometheus is forwarded to `localhost:19090` instead.

Deleting the `PhysicalMeasurement` also deletes its `ScrapeConfig`.
