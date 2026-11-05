package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	chapi "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type MeasurementDevicePhysicalMeasurementCombination struct {
	MeasurementDevice    *chapi.MeasurementDevice
	PhysicalMeasurements *chapi.PhysicalMeasurement
}

type stressTestFlags struct {
	mibPath                                    string
	namespace                                  string
	nbMeasurementDevices                       int
	nbPhysicalMeasurementPerMeasurementDevices int
	nbDataCenterResourcePerMeasurement         int
	delay                                      time.Duration
	cleanup                                    bool
}

const (
	defaultMibPath                                    = "./dev/mibs/TNO-PDU-MIB.txt"
	defaultNamespace                                  = "chantico"
	defaultNbMeasurementDevices                       = 2
	defaultNbPhysicalMeasurementPerMeasurementDevices = 4
	defaultNbDataCenterResourcePerMeasurement         = 3
	defaultDelay                                      = 100 * time.Millisecond
)

func parseFlags() stressTestFlags {
	cfg := stressTestFlags{}

	flag.StringVar(&cfg.mibPath, "mib-path", defaultMibPath, "path to the MIB file")
	flag.StringVar(&cfg.namespace, "namespace", defaultNamespace, "namespace for the resources")
	flag.IntVar(&cfg.nbMeasurementDevices, "measurement-devices", defaultNbMeasurementDevices, "number of MeasurementDevices to generate")
	flag.IntVar(&cfg.nbPhysicalMeasurementPerMeasurementDevices, "physical-measurements-per-device", defaultNbPhysicalMeasurementPerMeasurementDevices, "number of PhysicalMeasurements per MeasurementDevice")
	flag.IntVar(&cfg.nbDataCenterResourcePerMeasurement, "datacenter-resources-per-measurement", defaultNbDataCenterResourcePerMeasurement, "number of DataCenterResources per PhysicalMeasurement")
	flag.DurationVar(&cfg.delay, "delay", defaultDelay, "delay between create requests (e.g. 100ms, 1s)")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "cleanup resources")

	flag.Parse()

	return cfg
}

func createKubernetesClient() client.Client {
	// Build scheme and add our API types
	scheme := runtime.NewScheme()
	if err := chapi.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add API to scheme: %v\n", err)
	}

	// Get kubeconfig (in-cluster or KUBECONFIG)
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("unable to get kubeconfig: %v\n", err)
	}

	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %v\n", err)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		log.Fatalf("failed to load kubeconfig: %v\n", err)
	}
	clusterName := rawConfig.CurrentContext
	if ctx, exists := rawConfig.Contexts[clusterName]; exists {
		clusterName = ctx.Cluster
	}

	log.Printf("Connecting to cluster: %s\n", clusterName)
	log.Print("Do you agree? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		log.Fatal("Operation cancelled by user")
	}

	log.Printf("Connected to cluster: %s\n", clusterName)
	return client
}

func deleteResourcesByPattern(ctx context.Context, c client.Client, namespace string, pattern string, resourceList client.ObjectList) error {
	if err := c.List(ctx, resourceList, client.InNamespace(namespace)); err != nil {
		return err
	}

	items := reflect.ValueOf(resourceList).Elem().FieldByName("Items")

	if !items.IsValid() {
		return fmt.Errorf("resource list has no Items field")
	}

	for i := range items.Len() {
		item := items.Index(i).Addr().Interface().(client.Object)
		if strings.HasPrefix(item.GetName(), pattern) {
			if err := c.Delete(ctx, item); err != nil {
				log.Printf("Failed to delete %s: %v", item.GetName(), err)
			}
		}
	}
	return nil
}

func main() {
	cfg := parseFlags()

	client := createKubernetesClient()
	ctx := context.Background()

	if cfg.cleanup {
		log.Println("Cleaning up resources...")
		if err := deleteResourcesByPattern(ctx, client, cfg.namespace, "stresstest-md", &chapi.MeasurementDeviceList{}); err != nil {
			log.Printf("Failed to delete MeasurementDevices: %v", err)
		}
		if err := deleteResourcesByPattern(ctx, client, cfg.namespace, "stresstest-pm", &chapi.PhysicalMeasurementList{}); err != nil {
			log.Printf("Failed to delete PhysicalMeasurements: %v", err)
		}
		if err := deleteResourcesByPattern(ctx, client, cfg.namespace, "stresstest-dcr", &chapi.DataCenterResourceList{}); err != nil {

			log.Printf("Failed to delete DataCenterResources: %v", err)
		}
		return
	}

	// Generate the resources
	mds := generateMeasurementDevices(cfg.namespace, cfg.mibPath, cfg.nbMeasurementDevices)
	pms, mdpms := generatePhysicalMeasurements(cfg.namespace, mds, cfg.nbPhysicalMeasurementPerMeasurementDevices)
	dcrs := generateDataCenterResources(cfg.namespace, mdpms, cfg.nbDataCenterResourcePerMeasurement)

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
