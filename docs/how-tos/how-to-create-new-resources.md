---
title: "How to create new resources"
menu:
  main:
    parent: howto
    weight: 40
---

This guide describes how to add a new Kubernetes custom resource (CRD) to Chantico using Kubebuilder.

## Prerequisites

1. Install `kubebuilder`:
```bash
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/
```

1. Make sure your local environment is set up:
[How to set up the local development environment](how-to-setup-the-local-development-environment.md)

## Create the API and controller scaffolding

1. Generate the resource scaffolding:
```bash
kubebuilder create api --group chantico --version v1alpha1 --kind <RESOURCE_TYPE>
```

1. Remove the generated integration tests (these are not used in this repo):
```bash
rm internal/controller/suite_test.go internal/controller/<resource_type>_controller_test.go
```

## Define the schema

1. Update the Go types in `api/v1alpha1/<resource>_types.go`:
- Add the `Spec` fields for the desired state.
- Add the `Status` fields for observed state.
- Add validation markers if needed.

1. Regenerate code and manifests:
```bash
make build
```

## Implement controller behavior

1. Update the controller in `internal/controller/<resource>_controller.go`:
- Implement reconcile logic.
- Add required RBAC markers for the CRD.

1. Add or adjust tests in `internal/<resource>/` as needed.

## Apply to a cluster (optional)

If you want to test against a running dev cluster:

```bash
make install
```

This installs the CRDs into the current kube-context.
