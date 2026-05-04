---
title: "How to run the webapp"
menus:
  main:
    parent: howto
    weight: 40
---

## Webapp

Chantico provides a webapp that visualizes the parent-child relationships of DataCenterResource objects. It requires access to a Kubernetes cluster that contains the CRD DataCenterResource.

### TL;DR
```sh
# Run webapp
go run cmd/webapp/main.go
```

### Configuration

We currently allow environment variables to configure the webapp:
- PORT (default: 8080, port number for http server)
- KUBECONFIG (default: ~/.kube/config, path to kubernetes config)

### Example

```sh
# 1. Add the CRD DataCenterResource to your cluster
kubectl apply --file config/crds/bases/chantico.ci.tno.nl_datacenterresources.yaml

# 2. Add CR DataCenterResource to your cluster. You may use this example file.
kubectl apply --file config/samples/example-webapp-demo.yaml 

# 3. Run the webapp. Open in webbrowser.
go run cmd/webapp/main.go
```


### Limitations

The current implementation is basic. There is currently no visual distinction between making a reference to an existing parent, and referencing a non-existing parent. It also doesn't show error messages from the controller.


