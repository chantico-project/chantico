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
- make version 4.3+
- docker version v17.03+
- kubectl version v0.30.0+
- helm version 3.19+

Other useful binaries are versioned in the Makefile. These will be installed automatically when needed by the Makefile and are installed into the local `<root-project>/bin` folder.


### Creating the environment

Set up the following environment variables (this can be automated using [direnv](https://direnv.net/))

```bash
export CHANTICO_PROMETHEUS_SERVICE_HOST="localhost"
export CHANTICO_PROMETHEUS_SERVICE_PORT="19090"
export CHANTICO_PERSISTENT_VOLUME_CLAIM_NAME="chantico-persistent-volume-claim"
export CHANTICO_DATA_PATH=".chantico-persistent-volume"
```

The controller will run locally on your computer. The controller will talk to a Kubernetes cluster (typically KinD) and other dependencies like a timeseries database. Everything will therefore run inside the Kubernetes cluster, except for the controller, when developing. A typical flow is:

```bash
make cluster-up         # start up the kind cluster
make cluster-configure  # configures the manifests in the kind cluster
make run                # run the controller locally; this is blocking
```

Take into account that spinning up a Kubernetes cluster may take some time, and additionally having the pods to startup as well. Our experience is that it will take less than 1-2 minutes to setup.

You should now have access to:

- [Filebrowser](localhost:18888) - username and password are both `admin`
- [Prometheus](localhost:19090)
- [SNMP Exporter](localhost:19116)


#### Running a demo

After setting up the local development environment, you are ready to run the demo in [How to run the mock snmp device](how-to-run-the-mock-snmp-device.md).

### Removing the environment

To stop the environment we have the following commands:

```bash
make cluster-down   # stops the kind cluster, but keeps the data in the volume
make cluster-clean  # sotps the kind cluster, and removes the data
```
