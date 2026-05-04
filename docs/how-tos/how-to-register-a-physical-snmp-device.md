---
title: "How to register a physical snmp device"
menus:
  main:
    parent: howto
    weight: 30
---

`PhysicalMeasurement` does not parse MIBs. It links a concrete device IP to an existing `MeasurementDevice` (SNMP module/auth definition) and writes Prometheus scrape config, then reloads Prometheus.
In our First use-case (see `goal.md`) this corresponds to the `createPDU1` and `createPDU2` phases.

1. Ensure a matching `MeasurementDevice` exists (see `how-to-register-an-snmp-device-type.md`).
1. Create the PhysicalMeasurement matching the required type of PhysicalMeasurement
  1. Create a `physical_measurement.yaml` file

  ```yaml
  apiVersion: chantico.ci.tno.nl/v1alpha1
  kind: PhysicalMeasurement
  metadata:
    labels:
      app.kubernetes.io/name: chantico
      app.kubernetes.io/managed-by: kustomize
    name: physicalmeasurement-pdu1-out
    namespace: chantico
  spec:
    ip: 10.5.1.1
    serviceId: dee263f8-50e0-11f0-8cb5-00155d8a81e1 # This can be any type of UUID
    measurementDevice:  schleifenbauer-out # This has to be a currently valid MeasurementDevice name
  ```
  1. Apply the yaml file
  ```sh
  kubectl apply -f physical_measurement.yaml
  ```
1. Verify the new device setting
  1. Port-forward Prometheus
  ```sh
  kubectl port-forward -n chantico deployment/chantico-prometheus 9090:9090
  ```
  1. Check that the config (http://localhost:9090/targets)
  1. If you are using `./dev/port-forward.sh`, Prometheus is forwarded to `localhost:19090` instead.
  1. If running the controller locally, ensure the port-forward and env vars are set so the operator can call the reload endpoint:
  ```sh
  export CHANTICO_PROMETHEUS_SERVICE_HOST="localhost"
  export CHANTICO_PROMETHEUS_SERVICE_PORT="19090"
  ```
