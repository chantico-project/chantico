---
title: "Technical proposal"
weight: 10
main:
  parent: technical
---

## Technical proposal

The idea behind chantico is to use the kubernetes control plane as a basis to have a fully declarative approach to the energy domain control.
To make this happen Chantico is built as a [kubernetes controller](https://kubernetes.io/docs/concepts/architecture/controller/) operating over a set [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

### Components

#### chantico-filebrowser

The `chantico-filebrowser` is a kubernetes deployment living in the `chantico` namespace.
It allows to add configuration files via drag and drop (e.g. uploading MIB files for the registration of a PDU).

#### chantico-snmp

The `chantico-snmp` is a kubernetes deployment living in the `chantico` namespace.
It hosts an `snmp_exporter` instance that query devices using the SNMP format and exposes a prometheus compatible format.

#### chantico-prometheus

The `chantico-snmp` is a kubernetes service living in the `chantico` namespace.
It hosts an `prometheus` that scrapes devices via `chantico-snmp`.


## Technical choices

The backbone of chantico was implemented using [operator-sdk](https://sdk.operatorframework.io/).

### Documentation

This repository aims to be stand-alone.
This is why we use [plantuml](https://plantuml.com/) to write diagrams, as its text-based approach allows to store directly in this repository and iterate over design with gitlab project management tooling (issues, merge requests...)

### Programming language

To seamlessly interoperate with kubernetes the [go](https://go.dev/) programming language was chosen.

#### Go code

To avoid the [short-comings](https://en.wikipedia.org/wiki/Object%E2%80%93relational_impedance_mismatch) of ORMs an approach based on generating idiomatic go code directly from annotated SQL queries have been prefered.
To do this we use the [sqlc](https://sqlc.dev/) library.

### That does not work on my machine

To avoid the "it does not work" on my machine we provide a [nix-flake](https://wiki.nixos.org/wiki/Flakes) to set-up your development environment.
Although this is not strictly required this is encouraged to work on the project.

### Testing

#### Philosophy

**Chantico** is designed to serve as the glue between many components that run on and depend on Kubernetes.
Because of this, integration and end-to-end testing can be costly and significantly slow down the development cycle—due to long-running CI jobs and the complexity of setting up a proper development environment.

To address bugs that go beyond the scope of unit testing, we aim to invest in robust automatic logging that will be explained in its own section.

To keep testing lightweight and efficient, we follow these rules regarding what is allowed and disallowed in tests:

**Allowed in tests:**
- Creation of temporary directories and files
- Mocking the Kubernetes client
- Modifying OS environment variables

**Disallowed in tests:**
- Spinning up a Kubernetes instance
- Spinning up service instances (e.g., PostgreSQL, etc.)

#### Table-Driven Testing

In line with Go’s philosophy of simplicity, we use the standard `testing` package from the Go library.
When appropriate, we design tests using **table-driven testing**, following this format:

```go
func TestInitializeFinalizer(t *testing.T) {
    testCases := map[string]struct {
        Case     *chantico.MeasurementDevice
        Expected []string
    }{
        "empty finalizer": {
            Case: &chantico.MeasurementDevice{
                ObjectMeta: metav1.ObjectMeta{
                    Finalizers: []string{},
                }},
            Expected: []string{chantico.SNMPUpdateFinalizer},
        },
        "already initialized": {
            Case: &chantico.MeasurementDevice{
                ObjectMeta: metav1.ObjectMeta{
                    Finalizers: []string{"test"},
                }},
            Expected: []string{"test", chantico.SNMPUpdateFinalizer},
        },
    }

    for name, tc := range testCases {
        t.Run(name, func(t *testing.T) {
            InitializeFinalizer(tc.Case, nil)
            if !equalStringSlices(tc.Expected, tc.Case.ObjectMeta.Finalizers) {
                t.Errorf("InitializeFinalizer(%#v) = %#v, want %#v\n", tc, tc.Case.ObjectMeta.Finalizers, tc.Expected)
            }
        })
    }
}
```

#### Running the tests

To run the tests just launch the following command:

```bash
 go test -v ./internal/...
```

### Logging

Coming soon.

### CI/CD

We use GitLab CI to build Docker images for Chantico components, including the manager, Goose for Postgres migrations and SNMP mock.
Additionally, formatting, tests and coverage are run. Pages are also deployed from GitLab CI.

