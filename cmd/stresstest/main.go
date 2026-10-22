package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	chapi "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	mibPath                                    = "./dev/mibs/TNO-PDU-MIB.txt"
	nbMeasurementDevices                       = 2
	nbPhysicalMeasurementPerMeasurementDevices = 4
	nbDataCenterResourcePerMeasurement         = 3

	delay = 100 * time.Millisecond
)

type MeasurementDevicePhysicalMeasurementCombination struct {
	MeasurementDevice    *chapi.MeasurementDevice
	PhysicalMeasurements *chapi.PhysicalMeasurement
}

func createKubernetesClient() client.Client {
	// Build scheme and add our API types
	scheme := runtime.NewScheme()
	if err := chapi.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add API to scheme: %v\n", err)
		os.Exit(1)
	}

	// Get kubeconfig (in-cluster or KUBECONFIG)
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kubernetes client: %v\n", err)
		os.Exit(1)
	}
	return client
}

func main() {
	client := createKubernetesClient()
	ctx := context.Background()

	// Generate the resources
	mds := generateMeasurementDevices(mibPath, nbMeasurementDevices)
	pms, mdpms := generatePhysicalMeasurements(mds, nbPhysicalMeasurementPerMeasurementDevices)
	dcrs := generateDataCenterResources(mdpms, nbDataCenterResourcePerMeasurement)

	fmt.Printf("Generated %d MeasurementDevices, %d PhysicalMeasurements, and %d DataCenterResources\n", len(mds), len(pms), len(dcrs))

	seen := map[string]int{}
	for _, d := range dcrs {
		seen[d.Name]++
	}
	dups := 0
	for n, c := range seen {
		if c > 1 {
			fmt.Printf("duplicate name: %s (x%d)\n", n, c)
			dups += c - 1
		}
	}
	fmt.Printf("total=%d unique=%d duplicates=%d\n", len(dcrs), len(seen), dups)

	// Apply the resources
	for _, md := range mds {
		if err := client.Create(ctx, &md); err != nil {
			log.Printf("Failed to create %s: %v", md.Name, err)
		}
		time.Sleep(delay)
	}

	for _, pm := range pms {
		if err := client.Create(ctx, &pm); err != nil {
			log.Printf("Failed to create %s: %v", pm.Name, err)
		}
		time.Sleep(delay)
	}

	for _, dcr := range dcrs {
		if err := client.Create(ctx, &dcr); err != nil {
			log.Printf("Failed to create %s: %v", dcr.Name, err)
		}
		time.Sleep(delay)
	}
}
