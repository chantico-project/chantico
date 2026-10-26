---
title: "How to register data center resources"
menus:
  main:
    parent: howto
    weight: 30
---

In Chantico, one of the custom resources that can be provided is the data 
center resource. This resource defines which equipment and customer-facing 
services live in your cloud, such as PDUs, bare metals and VMs. The resource 
has relations to physical measurements, which define where the device can be 
monitored, such as IP address to scrape, and has a further reference to the 
measurement device, which describes how to monitor the type/brand of resource.

The data center resource also defines relationships with parent devices. A bare 
metal server can be connected to one or more PDUs, for example, and PDUs may 
have multiple dependent bare metals. Together, this forms a resource graph.

In order to use the graph correctly during aggregation and other calculation, 
the graph must be acyclic, such that no resource is both the parent of another 
resource and the other way around, or indirectly (grand-...)parent through 
other resources.

To create data center resources, have a look at 
`config/samples/chantico_v1alpha1_datacenterresource.yaml`.

1. Apply the file to your cluster:
  ```sh
kubectl apply -n chantico -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```
1. Edit the file to add a parent to one of the PDUs:
  ```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: DataCenterResource
metadata:
  labels:
    app.kubernetes.io/name: chantico
    app.kubernetes.io/managed-by: kustomize
  name: datacenterresource-pdu2
spec:
  type: pdu
  parents:
    - name: datacenterresource-misd-gbm-01
      coefficientFrom:
        value: "1"
  physicalMeasurements:
    - physicalMeasurement-pdu2-out
```
1. Apply the file again:
  ```sh
kubectl apply -n chantico -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```
1. Check the resource:
  ```sh
kubectl describe -n chantico datacenterresource datacenterresource-pdu2
```
  Notice that the status of the resource is updated to have a validation 
  message.
1. Revert your changes and apply the file again, to check that the validation 
   message disappears.

## Set energy metrics and coefficients

Root resources must define `energyMetricFrom`. Each parent listed by a child
must define `coefficientFrom`. Both fields use the same selector: provide the
PromQL expression inline with `value`, or load it from a ConfigMap or Secret
with `valueFrom`. For example, an inline PDU metric is:

```yaml
energyMetricFrom:
  value: 'tnoPduPowerValue{job="tno"}'
```

An inline coefficient on a parent reference is:

```yaml
parents:
  - name: datacenterresource-pdu1
    coefficientFrom:
      value: "1"
```

## Reuse PromQL with ConfigMap templates

Use a ConfigMap template when the same PromQL expression is needed for more
than one resource and only a few labels or values differ. Chantico reads the
template from the ConfigMap in the same namespace as the `DataCenterResource`,
renders it using Go template syntax, and uses the result as the metric or
coefficient expression. You can also put template text directly in `value`;
provide `templateParameters` whenever placeholders need substituting.

1. Create a ConfigMap with the PromQL expression under a named key. Template
   values use the parameter name, for example `{{ .job }}`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tno-pdu-energy-metric-template
  namespace: chantico
data:
  template: |
    tnoPduPowerValue{job="{{ .job }}"}
```
2. Reference the template from `spec.energyMetricFrom` or
  `spec.parents[].coefficientFrom`. Each entry in `templateParameters`
  supplies a named template value:

```yaml
apiVersion: chantico-project.github.io/v1alpha1
kind: DataCenterResource
metadata:
  name: datacenterresource-pdu1
  namespace: chantico
spec:
  type: pdu
  energyMetricFrom:
    valueFrom:
      configMapKeyRef:
        name: tno-pdu-energy-metric-template
        key: template
    templateParameters:
      - name: job
        value: tno
```

Template parameter values can be provided directly with `value`, or loaded with
`valueFrom.configMapKeyRef` or `valueFrom.secretKeyRef`. All resources 
referenced must be in the same namespace as the DataCenterResource. 
IN case of a missing parameter or an invalid key reference, the resource 
will not reconcile. Errors will be attached to the DataCenterResource 
which can be checked with `kubectl describe`.

## Future steps

The current state of the data center resource may change in a future 
development iteration to allow more information to be added about which 
specific time series to monitor from a parent resource in order to aggregate 
and combine measurements. For example, if a bare metal is connected to a PDU on 
specific slots, we would want to include only specific walks from that PDU in 
this resource.
