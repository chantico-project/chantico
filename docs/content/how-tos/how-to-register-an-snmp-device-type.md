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
    1. If you have created the cluster using the [local development setup howto](how-to-setup-the-local-development-environment.md), you can use `make cluster-mibs` to copy all the MIB files in the `dev/mibs` folder into the persistent volume claim.
    1. Otherwise, browse to the [filebrowser](http://localhost:18888). Either use the port-forward from the kind-config (if you have a KinD cluster) or use the port-forward command:
        ```sh
        kubectl port-forward -n chantico deployment/chantico-filebrowser 18888:80
        ```
    1. Login with (user: admin, password: admin)
    1. Upload your MIBS files in `snmp/mibs`
1. Create the MeasurementDevice matching the required type of MeasurementDevice
    1. Create a `measurement_device.yaml` file

        ```yaml
          apiVersion: chantico-project.github.io/v1alpha1
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

## Metric name disambiguation

If two metrics have the same name, there are two methods to disambiguate. Let's take the example of the `tnoPduEnergyValue` metric present in both `./dev/mibs/TNO-ANOTHERPDU-MIB.txt`, and `./dev/mibs/TNO-PDU-MIB.txt`:

1. the user can fully qualify the "path" within the MIB tree via human readable language `TNO-PDU-MIB::tnoPduEnergyValue`
1. the user can fully qualify the "path" within the MIB tree via the OID `1.3.6.1.4.99999.1`

