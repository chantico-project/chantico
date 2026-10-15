package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	chapi "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ips = []string{"chantico-snmp-mock:1161", "chantico-snmp-mock-2:1161"}

func generatePhysicalMeasurement(measurementDevice chapi.MeasurementDevice, idx int, ips []string) chapi.PhysicalMeasurement {
	if len(ips) == 0 {
		fmt.Fprintf(os.Stderr, "no ip provided")
		os.Exit(1)
	}

	name := fmt.Sprintf("stresstest-pm-%d-%d", time.Now().Unix(), idx)
	ip := ips[rand.Intn(len(ips))]

	pm := chapi.PhysicalMeasurement{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PhysicalMeasurement",
			APIVersion: "chantico/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "chantico",
		},
		Spec: chapi.PhysicalMeasurementSpec{
			Ip:                ip,
			MeasurementDevice: measurementDevice.Name,
		},
	}
	return pm
}

func generatePhysicalMeasurements(measurementDevices []chapi.MeasurementDevice, nbMeasurementPerPhysicalMeasurement int) []chapi.PhysicalMeasurement {
	if nbMeasurementPerPhysicalMeasurement <= 0 || len(measurementDevices) == 0 {
		return nil
	}

	total := len(measurementDevices) * nbMeasurementPerPhysicalMeasurement
	out := make([]chapi.PhysicalMeasurement, 0, total)

	idx := 0
	for _, md := range measurementDevices {
		for range nbMeasurementPerPhysicalMeasurement {
			pm := generatePhysicalMeasurement(md, idx, ips)
			out = append(out, pm)
			idx++
		}
	}
	return out
}
