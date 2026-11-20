---
title: "How to run the webapp"
menus:
  main:
    parent: howto
    weight: 40
---

## Webapp

Chantico provides a webapp that visualizes the parent-child relationships of DataCenterResource objects. It requires access to a Kubernetes cluster that contains the CRD DataCenterResource.

### Run the webapp from the Helm chart

The Helm chart does not deploy the webapp by default. Enable it when installing Chantico:

```sh
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico --create-namespace --set webapp.enabled=true
```

The webapp Service is internal to the cluster by default. Forward it to your machine and open [http://localhost:8080](http://localhost:8080):

```sh
kubectl port-forward -n chantico service/chantico-visualisation-webapp 8080:80
```

#### Expose the webapp through the Gateway API

The chart can create an `HTTPRoute` for the webapp. This requires the [Gateway API](https://gateway-api.sigs.k8s.io/) CRDs, a Gateway API implementation and an existing `Gateway` that allows routes from the Chantico namespace:

```sh
helm upgrade --install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico \
  --set webapp.enabled=true \
  --set webapp.httpRoute.enabled=true \
  --set webapp.httpRoute.parentRefs[0].name=my-gateway \
  --set webapp.httpRoute.parentRefs[0].namespace=gateway-system \
  --set webapp.httpRoute.hostnames[0]=chantico.example.com
```

The webapp has no authentication and its service account can read all DataCenterResource objects in the cluster. Only expose it on trusted networks, or add appropriate authentication at the Gateway.

In the local development environment, `make cluster-gateway` installs [Envoy Gateway](https://gateway.envoyproxy.io/), creates the `chantico-dev` Gateway from `dev/k8s/gateway.yaml` and reruns `cluster-configure` with the route enabled. 
The webapp is then available at [http://localhost:18080](http://localhost:18080). 

#### Use a locally built image in the development cluster

When using the local development environment, `make cluster-configure` deploys 
the webapp with the image from GHCR by default
(`ghcr.io/chantico-project/images/chantico-visualisation-webapp:<current-version>`). 
During development on the webapp, the image can be built and loaded into the 
Kind cluster manually:

```sh
make docker-build-visualisation-webapp cluster-load-visualisation-webapp cluster-configure \
  VISUALISATION_WEBAPP_REPOSITORY=chantico-visualisation-webapp \
  VISUALISATION_WEBAPP_TAG=dev
```

After rebuilding the image with the tag, load it and restart the
deployment to pick up the changes:

```sh
make docker-build-visualisation-webapp cluster-load-visualisation-webapp \
  VISUALISATION_WEBAPP_REPOSITORY=chantico-visualisation-webapp \
  VISUALISATION_WEBAPP_TAG=dev
kubectl rollout restart -n chantico deployment/chantico-visualisation-webapp
```

### TL;DR
```sh
# Run webapp
go run cmd/webapp/main.go
```

### Configuration

We currently allow environment variables to configure the webapp:
- PORT (default: 8080, port number for http server)
- KUBECONFIG (path to kubernetes config; when unset, the default loading rules apply, i.e. ~/.kube/config, or in-cluster config when running in a cluster)

### Example

```sh
# 1. Add the CRD DataCenterResource to your cluster
kubectl apply --file config/crds/bases/chantico-project.github.io_datacenterresources.yaml

# 2. Add CR DataCenterResource to your cluster. You may use this example file.
kubectl apply --file config/samples/example-webapp-demo.yaml 

# 3. Run the webapp. Open in webbrowser.
go run cmd/webapp/main.go
```


### Limitations

The current implementation is basic. There is currently no visual distinction between making a reference to an existing parent, and referencing a non-existing parent. It also doesn't show error messages from the controller.


