---
title: "SNMP configuration with measurement device resource"
weight: 20
---

This document describes the `MeasurementDevice` custom resource definition (CRD) 
that is managed by the Chantico controller. It explains the design and data 
model, how the resource is structured and used by the controller to perform 
updates to the SNMP exporter configuration.

## Introduction

Chantico is built on the foundation that most information useful for chantico is 
provided in custom resources. This allows for a declarative approach to managing 
the devices in a data center that are relevant for the energy usage domain. 
A core data source for chantico are devices that expose their metrics via the 
Simple Network Management Protocol (SNMP), which is a widely used protocol for 
monitoring and managing physical devices connected to a network.

One problem is that SNMP devices often have different ways of exposing their 
metrics. An SNMP device is a physical device that can be monitored for energy 
usage, such as a PDU or a bare metal server. Each brand or version of a device 
may support different SNMP versions and different categories of management data 
to expose. For such systems, there often exist vendor-provided, proprietary 
files that describe how to access particular fields. This files are known as 
Management Information Base (MIB) files. From the hierarchy described in the MIB 
file, it is possible to deduce a range of object identifiers (OIDs) that are 
relevant for energy usage monitoring, such as PDU outlet power or voltage, or 
bare metal power consumption. These MIB files need to be provided separately to 
chantico to allow the SNMP exporter to perform the correct queries.

The `MeasurementDevice` custom resource allows to describe a type of SNMP 
device, including the authentication parameters and so-called walks. The walks 
need to correspond to some field in the MIB file. They could refer to a specific 
OID, but more commonly they refer to a name of a field or table in the MIB file. 
The SNMP exporter can translate such field names, also known as *walks*, into 
the corresponding OIDs. The SNMP exporter can then configure itself to perform 
"bulk walk" queries to effieiciently obtain the most recent measurement value 
for the desired metrics.

## Fields

| Field | Type | Description |
|---|---|---|
| `metadata.name` | `string` | The name of the resource. This is used to reference the resource in instances of other custom resource definitions. |
| `spec.auth` | `SNMPAuth` | The authentication parameters for the SNMP device. This is based on the configuration of the [SNMP exporter](https://github.com/prometheus/snmp_exporter/blob/main/config/config.go) |
| `spec.walks` | list of `string` | The list of walks that are to be requested from devices using this type. Each walk corresponds to an OID, field or table in the MIB file, and may be namespaced to the base MIB file name. |

Note that the OIDs may still be translated into field names once the metrics are 
collected by the SNMP exporter (which does translation before or after 
configuration depending on how specific the OIDs are, if they can be merged for 
a bulk walk or not, etc.), due it having access to the MIB files. After being 
scraped by Prometheus, the walk names become time series metric names to use in 
queries.
