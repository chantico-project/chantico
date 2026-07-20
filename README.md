# Chantico - energy controller

<img src="docs/assets/logo/chantico.png" width="150" height="150">

## Description

In Aztec religion, [Chantico](https://en.wikipedia.org/wiki/Chantico) is the 
deity who reigns over the fires of hearths and fire stoves. If you would 
substitute hearths with datacenter bare metals, Chantico would have similar 
ruling power over the energy flowing through the data center resources in our 
context. Chantico is a [K8s SDK operator](https://sdk.operatorframework.io) 
project handling the monitoring of power usage of devices, such as PDUs and 
bare metal servers monitored with SNMP but we also envision monitoring VMs 
running on hypervisors and pods in clusters.

## Getting Started

How-to guides can be found in the `/docs` folder and on our [documentation 
website](https://chantico-project.github.io/chantico/).

For a quick start, install Chantico on your k8s cluster using:

```bash
helm install chantico oci://ghcr.io/chantico-project/charts/chantico -n chantico # Latest version
```

For more information have a look at the following [installation 
guide](https://chantico-project.github.io/chantico/how-tos/how-to-install-chantico/). For a local setup of Chantico, please 
have a look at the following 
[guide](https://chantico-project.github.io/chantico/how-tos/how-to-setup-the-local-development-environment/).

### Prerequisites

To install the Helm-based deployment:
- helm version 3.19+.
- kubectl version v1.11.3+.

Additionally, for development:
- go version v1.25.11+.
- make version 4.3+.
- docker version 17.03+.

If not using local development using kind:
- Access to a Kubernetes v1.11.3+ cluster.

## Contributing

We welcome issues, discussions, and pull requests based on the former
two. Please have a look at our [contribution
guidance](https://github.com/chantico-project/.github/blob/main/CONTRIBUTING.md).

### Using DCO sign-off

Contributions to Chantico requires Developer's Certificate of origin (DCO), as stated in our [contribution guide](https://github.com/chantico-project/.github/blob/main/CONTRIBUTING.md#developer-certificate-of-origin). This can be configured with git CLI in at least the following ways:

1. **Plain option:** Use the default `git commit -s` command when committing.
2. **Commit message hook:** In order to be able to commit with `commit -m` while also using DCO, add a git message hook for this cloned repository to add the sign-off automatically. Add the following lines to your local repository's `.git/hooks/commit-msg`:

```bash
#!/bin/sh
SIGNATURE="Signed-off-by: `git config --global --get user.name` <`git config --global --get user.email`>"
grep -qs "^${SIGNATURE}" "$1" || echo "\n${SIGNATURE}" >> "$1"
```
3. **Git aliases:** Another option to configure this automatically is with git aliases (shortcuts). In this way you can add the `-s` field to the commit command. Add the following lines to your `~/.gitconfig`.

```
[alias]
  cmsg = commit -s -m
  camend = commit -s --amend
```

To add sign-offs retroactively, use git rebase with signoff option, like so `git rebase --signoff HEAD^^`. Use as many `^` as there are commits in your pull requests.

## Code of Conduct

Please consider the guidelines in the [Code of 
Conduct](https://github.com/chantico-project/.github/blob/main/CODE_OF_CONDUCT.md) when 
participating in our shared environment.

## License

Copyright 2025-2026 TNO.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

