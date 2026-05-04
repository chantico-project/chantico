---
title: "How to set-up the local development environment"
menus:
  main:
    parent: howto
    weight: 30
---

### Prerequisites

The development currently supports [WSL2](https://github.com/microsoft/WSL) and UNIX based environment.

It requires the following packages:

- go version v1.24.13+
- kind version v0.30.0+
- docker version v17.03+
- helm version 3.19+
- make version 4.3+
- kubectl version v0.30.0+

### Installation

- Login your docker client:

  ```bash
  docker login ci.tno.nl
  ```

- To install the kind docker cluster, run:

  ```bash
  ./dev/setup.sh
  ```

- In a separate terminal, setup the port forward:

  ```bash
  ./dev/port-forward.sh
  ```
  
  Redo this command whenever you end it to help developing.

- Set up the following environment variables (this can be automated using [direnv](https://direnv.net/))

  ```bash
  export CHANTICO_PROMETHEUS_SERVICE_HOST="localhost"
  export CHANTICO_PROMETHEUS_SERVICE_PORT="19090"
  export CHANTICOVOLUMELOCATIONENV="$(kubectl get pv -o jsonpath='{range .items[?(@.spec.claimRef.name=="chantico-snmp-prometheus-volume-claim")]}{.spec.hostPath.path}{"\n"}{end}' | sed 's|/opt/local-path-provisioner|/tmp/chantico-local-path-data|')"
  export CHANTICOVOLUMECLAIMENV="chantico-snmp-prometheus-volume-claim"
  ```

  It might take a little while for the volume to show up, so redo the final 
  export or change the directory back and forth to reapply the direnv.

- Run the chantico controllers locally:

  ```bash
  make run
  ```

### Running a demo

After setting up the local development environment, you are ready to run the demo in [How to run the mock snmp device](how-to-run-the-mock-snmp-device.md).

### Teardown

To teardown a local installation of the kind cluster, run the script:

```bash
./dev/teardown.sh
```
