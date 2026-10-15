package main

import (
	// "context"
	// "log"
	// "time"
	"fmt"
	"os"

	chapi "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	mibPath                                    = "./dev/mibs/TNO-PDU-MIB.txt"
	nbMeasurementDevices                       = 100
	nbPhysicalMeasurementPerMeasurementDevices = 10
)

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
	// client := createKubernetesClient()
	// ctx := context.Background()

	// Generate the resources
	mds := generateMeasurementDevices(mibPath, nbMeasurementDevices)
	pms := generatePhysicalMeasurements(mds, nbPhysicalMeasurementPerMeasurementDevices)

	// Visualize resources
	for _, md := range mds {
		fmt.Printf("MD: %s, Walk: %v\n", md.Name, md.Spec.Walks)
	}
	for _, pm := range pms {
		fmt.Printf("PM: %s, MD: %s, Ip: %s\n", pm.Name, pm.Spec.MeasurementDevice, pm.Spec.Ip)
	}

	// Apply the resources
	// for _, md := range mds {
	// 	if err := client.Create(ctx, &md); err != nil {
	// 		log.Printf("Failed to create %s: %v", md.Name, err)
	// 	}
	// }
}
