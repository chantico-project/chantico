package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	chapi "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type MeasurementDevicePhysicalMeasurementCombination struct {
	MeasurementDevice    *chapi.MeasurementDevice
	PhysicalMeasurements *chapi.PhysicalMeasurement
}

type stressTestFlags struct {
	mibPath                                    string
	nbMeasurementDevices                       int
	nbPhysicalMeasurementPerMeasurementDevices int
	nbDataCenterResourcePerMeasurement         int
	delay                                      time.Duration
}

const (
	defaultMibPath                                    = "./dev/mibs/TNO-PDU-MIB.txt"
	defaultNbMeasurementDevices                       = 2
	defaultNbPhysicalMeasurementPerMeasurementDevices = 4
	defaultNbDataCenterResourcePerMeasurement         = 3
	defaultDelay                                      = 100 * time.Millisecond
)

func parseFlags() stressTestFlags {
	cfg := stressTestFlags{}

	flag.StringVar(&cfg.mibPath, "mib-path", defaultMibPath, "path to the MIB file")
	flag.IntVar(&cfg.nbMeasurementDevices, "measurement-devices", defaultNbMeasurementDevices, "number of MeasurementDevices to generate")
	flag.IntVar(&cfg.nbPhysicalMeasurementPerMeasurementDevices, "physical-measurements-per-device", defaultNbPhysicalMeasurementPerMeasurementDevices, "number of PhysicalMeasurements per MeasurementDevice")
	flag.IntVar(&cfg.nbDataCenterResourcePerMeasurement, "datacenter-resources-per-measurement", defaultNbDataCenterResourcePerMeasurement, "number of DataCenterResources per PhysicalMeasurement")
	flag.DurationVar(&cfg.delay, "delay", defaultDelay, "delay between create requests (e.g. 100ms, 1s)")
	flag.Parse()

	return cfg
}

func createKubernetesClient() client.Client {
	// Build scheme and add our API types
	scheme := runtime.NewScheme()
	if err := chapi.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add API to scheme: %v\n", err)
		os.Exit(1)
	}

	// Get kubeconfig (in-cluster or KUBECONFIG)
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("unable to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %v\n", err)
		os.Exit(1)
	}
	return client
}

func main() {
	cfg := parseFlags()

	client := createKubernetesClient()
	ctx := context.Background()

	// Generate the resources
	mds := generateMeasurementDevices(cfg.mibPath, cfg.nbMeasurementDevices)
	pms, mdpms := generatePhysicalMeasurements(mds, cfg.nbPhysicalMeasurementPerMeasurementDevices)
	dcrs := generateDataCenterResources(mdpms, cfg.nbDataCenterResourcePerMeasurement)

	log.Printf("Generated %d MeasurementDevices, %d PhysicalMeasurements, and %d DataCenterResources\n", len(mds), len(pms), len(dcrs))

	// Apply the resources
	for _, md := range mds {
		if err := client.Create(ctx, &md); err != nil {
			log.Printf("Failed to create %s: %v", md.Name, err)
		}
		time.Sleep(cfg.delay)
	}

	for _, pm := range pms {
		if err := client.Create(ctx, &pm); err != nil {
			log.Printf("Failed to create %s: %v", pm.Name, err)
		}
		time.Sleep(cfg.delay)
	}

	for _, dcr := range dcrs {
		if err := client.Create(ctx, &dcr); err != nil {
			log.Printf("Failed to create %s: %v", dcr.Name, err)
		}
		time.Sleep(cfg.delay)
	}
}
