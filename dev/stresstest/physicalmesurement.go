package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	chapi "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ips = []string{"chantico-snmp-mock:1161", "chantico-snmp-mock-2:1161"}

func generatePhysicalMeasurement(namespace string, measurementDevice chapi.MeasurementDevice, idx int, ips []string) chapi.PhysicalMeasurement {
	if len(ips) == 0 {
		log.Fatalf("no ip provided")
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
			Namespace: namespace,
		},
		Spec: chapi.PhysicalMeasurementSpec{
			Ip:                ip,
			MeasurementDevice: measurementDevice.Name,
			Type:              "snmp",
		},
	}
	return pm
}

func generatePhysicalMeasurements(namespace string, measurementDevices []chapi.MeasurementDevice, nbMeasurementPerPhysicalMeasurement int) ([]chapi.PhysicalMeasurement, []MeasurementDevicePhysicalMeasurementCombination) {
	if nbMeasurementPerPhysicalMeasurement <= 0 || len(measurementDevices) == 0 {
		return nil, nil
	}

	total := len(measurementDevices) * nbMeasurementPerPhysicalMeasurement
	out := make([]chapi.PhysicalMeasurement, 0, total)
	outCombinations := make([]MeasurementDevicePhysicalMeasurementCombination, 0, total)

	idx := 0
	for _, md := range measurementDevices {
		for range nbMeasurementPerPhysicalMeasurement {
			pm := generatePhysicalMeasurement(namespace, md, idx, ips)
			out = append(out, pm)
			outCombinations = append(outCombinations, MeasurementDevicePhysicalMeasurementCombination{
				MeasurementDevice:    &md,
				PhysicalMeasurements: &pm,
			})
			idx++
		}
	}
	return out, outCombinations
}
