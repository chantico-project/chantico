package config

import (
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Images struct {
	BusyBox       string
	SnmpGenerator string
	SnmpExporter  string
	Postgres      string
	Filebrowser   string
	Prometheus    string
}

type Config struct {
	Images         Images
	MountPath      string
	SnmpDeployment client.ObjectKey
}

func New() (Config, error) {

	// read variables from environment before startup
	// fail if configuration is incorrect

	var errs []error

	mountPath := os.Getenv("CHANTICOVOLUMELOCATIONENV")
	if mountPath == "" {
		errs = append(errs, fmt.Errorf("no mount path specified. CHANTICOVOLUMELOCATIONENV missing."))
	} else {
		// checking whether directory exists
		info, err := os.Stat(mountPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("CHANTICOVOLUMELOCATIONENV error checking directory: %s", mountPath))
		} else if !info.IsDir() {
			errs = append(errs, fmt.Errorf("CHANTICOVOLUMELOCATIONENV is not a directory: %s", mountPath))
		}

	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return Config{
		Images: Images{
			BusyBox:       "busybox:1.36.1-glibc",
			SnmpGenerator: "prom/snmp-generator:v0.29.0",
			SnmpExporter:  "ricardbejarano/snmp_exporter:0.26.0",
			Postgres:      "postgres:17.6",
			Filebrowser:   "filebrowser/filebrowser:v2.32.2",
			Prometheus:    "prom/prometheus:v3.7.3",
		},
		SnmpDeployment: client.ObjectKey{
			Name:      "chantico-snmp",
			Namespace: "chantico",
		},
		MountPath: mountPath,
	}, nil
}
