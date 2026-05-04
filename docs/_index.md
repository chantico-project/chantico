---
title: "Chantico"
menus:
  main:
    weight: -100
    identifier: "chantico-main"
    name: "Chantico"
---

Streamlining Energy Management for Cloud Operators.

{{< figure src="assets/logo/chantico.png" alt="" width="150" height="150" >}}

## Naming

> In Aztec religion, Chantico ("she who dwells in the house") is the deity reigning over the fires

As the aforecited extract of the Wikipedia page of [Chantico](https://en.wikipedia.org/wiki/Chantico), Chantico is reigning.
It therefore felt natural to call the energy domain controller developped within the MISD project according to that deity.

## Installation

[Please refer to the following document](how-tos/how-to-install-chantico.md)

## Local developer

This is the fastest way to iterate: run the controller locally and use port-forwards for cluster services.

1. Set up the local development environment:
[How to set up the local development environment](how-tos/how-to-setup-the-local-development-environment.md)
1. Run the SNMP mock demo end-to-end (including Prometheus):
[How to run the mock snmp device](how-tos/how-to-run-the-mock-snmp-device.md)

More tutorials and how to documents are found in the [How tos](how-tos/) section.

## Roadmap

[Milestones](roadmap/) are defined by the development team in collaboration with the workflow orchestrator team.
Relevant features are then developed to support the use cases defined in the milestones.

## Technical design

Some technical choices are documented in our [technical proposal](technical/) section.

## API documentation

If this is deployed in GitLab pages then you can find internal [API documentation here](./technical/api/index.html)
