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
    1. Create a `measurement_device.yaml` file. The SNMP authentication is 
       provided through `spec.authFrom`, which reads it from a `Secret` (see 
       [Providing SNMP authentication](#providing-snmp-authentication) for 
       other options):

        ```yaml
          apiVersion: v1
          kind: Secret
          metadata:
            name: example-snmp-auth
            namespace: chantico
          stringData:
            auth: |
              community: public
              version: 2
          ---
          apiVersion: chantico-project.github.io/v1alpha1
          kind: MeasurementDevice
          metadata:
            labels:
              app.kubernetes.io/name: chantico
              app.kubernetes.io/managed-by: kustomize
            name: example-measurement-device
            namespace: chantico
          spec:
            authFrom:
              valueFrom:
                secretKeyRef:
                  name: example-snmp-auth
                  key: auth
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

## Providing SNMP authentication

The SNMP Auth can either be provided directly in the `MeasurementDevice` resource using `spec.authFrom.value`, 
or it can be sourced from a `Secret` or `ConfigMap` using `spec.authFrom.valueFrom`. Regardless of 
the resolved value must be a valid YAML document containing the SNMP 
authentication parameters, using the same format as an entry under `auths` in 
the [SNMP exporter generator 
configuration](https://github.com/prometheus/snmp_exporter/tree/main/generator#file-format). 

The value can come from exactly one of three sources:

1. A `Secret` (recommended for credentials such as SNMPv3 passwords):
    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: example-snmp-v3-auth
      namespace: chantico
    stringData:
      auth: |
        version: 3
        security_level: authPriv
        username: monitor
        password: my-auth-password
        auth_protocol: SHA
        priv_protocol: AES
        priv_password: my-priv-password
    ---
    # in the MeasurementDevice
    spec:
      authFrom:
        valueFrom:
          secretKeyRef:
            name: example-snmp-v3-auth
            key: auth
    ```
1. A `ConfigMap`:
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: example-snmp-auth
      namespace: chantico
    data:
      auth: |
        community: public
        version: 2
    ---
    # in the MeasurementDevice
    spec:
      authFrom:
        valueFrom:
          configMapKeyRef:
            name: example-snmp-auth
            key: auth
    ```
1. Inline (only suitable for non-sensitive settings):
    ```yaml
    spec:
      authFrom:
        value: |
          community: public
          version: 2
    ```

## Metric name disambiguation

If two metrics have the same name, there are two methods to disambiguate. Let's take the example of the `tnoPduEnergyValue` metric present in both `./dev/mibs/TNO-ANOTHERPDU-MIB.txt`, and `./dev/mibs/TNO-PDU-MIB.txt`:

1. the user can fully qualify the "path" within the MIB tree via human readable language `TNO-PDU-MIB::tnoPduEnergyValue`
1. the user can fully qualify the "path" within the MIB tree via the OID `1.3.6.1.4.99999.1`

