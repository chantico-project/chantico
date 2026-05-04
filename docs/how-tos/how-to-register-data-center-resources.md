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
kubectl apply -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```
1. Edit the file to add a parent to one of the PDUs:
  ```yaml
apiVersion: chantico.ci.tno.nl/v1alpha1
kind: DataCenterResource
metadata:
  labels:
    app.kubernetes.io/name: chantico
    app.kubernetes.io/managed-by: kustomize
  name: datacenterresource-pdu2
  namespace: chantico
spec:
  type: pdu
  parents:
    - datacenterresource-misd-gbm-01
  physicalMeasurements:
    - physicalMeasurement-pdu2-out
```
1. Apply the file again:
  ```sh
kubectl apply -f config/samples/chantico_v1alpha1_datacenterresource.yaml
```
1. Check the resource:
  ```sh
kubectl describe -n chantico datacenterresource datacenterresource-pdu2
```
  Notice that the status of the resource is updated to have a validation 
  message.
1. Revert your changes and apply the file again, to check that the validation 
   message disappears.

## Future steps

The current state of the data center resource may change in a future 
development iteration to allow more information to be added about which 
specific time series to monitor from a parent resource in order to aggregate 
and combine measurements. For example, if a bare metal is connected to a PDU on 
specific slots, we would want to include only specific walks from that PDU in 
this resource.
