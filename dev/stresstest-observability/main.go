package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	chapi "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type metrics struct {
	mdCreated     prometheus.Gauge
	mdReconciled  prometheus.Gauge
	pmCreated     prometheus.Gauge
	pmReconciled  prometheus.Gauge
	dcrCreated    prometheus.Gauge
	dcrReconciled prometheus.Gauge
	latency       prometheus.Gauge
	kubeClient    client.Client
	namespace     string
}

func newMetrics(reg prometheus.Registerer, kubeClient client.Client, namespace string) *metrics {
	m := &metrics{
		mdCreated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "measurement_device_created",
			Help: "Observed number of measurement devices created",
		}),
		mdReconciled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "measurement_device_reconciled",
			Help: "Observed number of measurement devices reconciled",
		}),
		pmCreated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "physical_measurement_created",
			Help: "Observed number of physical measurements created",
		}),
		pmReconciled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "physical_measurement_reconciled",
			Help: "Observed number of physical measurements reconciled",
		}),
		dcrCreated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "datacenter_resources_created",
			Help: "Observed number of datacenter resources created",
		}),
		dcrReconciled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "datacenter_resources_reconciled",
			Help: "Observed number of datacenter resources reconciled",
		}),
		latency: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "latency_seconds",
			Help: "Kubernetes API query latency",
		}),
		kubeClient: kubeClient,
		namespace:  namespace,
	}

	reg.MustRegister(m.mdCreated, m.mdReconciled, m.pmCreated, m.pmReconciled, m.dcrCreated, m.dcrReconciled, m.latency)
	return m
}

func startMetricsServer(reg *prometheus.Registry, addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	go func() {
		log.Printf("metrics server listening on %s", addr)
		log.Fatal(http.ListenAndServe(addr, mux))
	}()
}

func observeResource(m *metrics, list client.ObjectList, createdGauge, reconciledGauge prometheus.Gauge) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := m.kubeClient.List(ctx, list, client.InNamespace(m.namespace)); err != nil {
		return err
	}
	elapsed := time.Since(start).Seconds()
	m.latency.Set(elapsed)

	// Extract items using reflection
	listValue := reflect.ValueOf(list).Elem()
	itemsField := listValue.FieldByName("Items")
	if !itemsField.IsValid() {
		return fmt.Errorf("Items field not found in list")
	}

	created := itemsField.Len()
	reconciled := 0

	for i := range created {
		item := itemsField.Index(i)
		generation := item.FieldByName("ObjectMeta").FieldByName("Generation").Int()
		observedGeneration := item.FieldByName("Status").FieldByName("ObservedGeneration").Int()

		// Check if ObservedGeneration matches Generation
		if observedGeneration != generation {
			continue
		}

		// Check if Ready condition is True
		conditionsField := item.FieldByName("Status").FieldByName("Conditions")
		if !conditionsField.IsValid() {
			continue
		}

		isReady := false
		for j := range conditionsField.Len() {
			condition := conditionsField.Index(j)
			condType := condition.FieldByName("Type").String()
			if condType == "Ready" {
				status := condition.FieldByName("Status").String()
				if status == "True" {
					isReady = true
					break
				}
			}
		}

		if isReady {
			reconciled++
		}
	}

	createdGauge.Set(float64(created))
	reconciledGauge.Set(float64(reconciled))
	return nil
}

func observe(m *metrics) {
	for {

		if err := observeResource(m, &chapi.MeasurementDeviceList{}, m.mdCreated, m.mdReconciled); err != nil {
			log.Printf("error listing MeasurementDevices: %v", err)
		}

		if err := observeResource(m, &chapi.PhysicalMeasurementList{}, m.pmCreated, m.pmReconciled); err != nil {
			log.Printf("error listing PhysicalMeasurements: %v", err)
		}

		if err := observeResource(m, &chapi.DataCenterResourceList{}, m.dcrCreated, m.dcrReconciled); err != nil {
			log.Printf("error listing DatacenterResources: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func createKubernetesClient(kubeconfig string) client.Client {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatal(err)
	}

	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		log.Fatal(err)
	}
	if err := chapi.AddToScheme(s); err != nil {
		log.Fatal(err)
	}

	c, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		log.Fatal(err)
	}

	return c
}

func main() {
	reg := prometheus.NewRegistry()

	c := createKubernetesClient("/kubeconfig")
	m := newMetrics(reg, c, "chantico")

	startMetricsServer(reg, ":2112")
	observe(m)
}
