package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const vmAttributionBase = `# HELP vm_attribution_coefficient Fraction of total system usage attributed to this VM.
# TYPE vm_attribution_coefficient gauge
`

var (
	hostName = envString("MOCK_PROMETHEUS_HOST_NAME", "prometheus-mock-bm-1")
	vmIDs    = envVMIDs("MOCK_PROMETHEUS_VM_IDS", []int{120, 121})
	port     = envString("MOCK_PROMETHEUS_PORT", "9090")
)

func envString(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

func envVMIDs(name string, defaultValues []int) []int {
	value := os.Getenv(name)
	if value == "" {
		return defaultValues
	}

	// Split the value by commas and whitespace, and convert to integers
	values := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(values) == 0 {
		log.Printf("invalid %s=%q; using defaults", name, value)
		return defaultValues
	}

	vmIDs := make([]int, 0, len(values))
	for _, value := range values {
		vmID, err := strconv.Atoi(value)
		if err != nil || vmID < 0 {
			log.Printf("invalid %s=%q; using defaults", name, os.Getenv(name))
			return defaultValues
		}
		vmIDs = append(vmIDs, vmID)
	}
	return vmIDs
}

func simulateVMAttribution(w http.ResponseWriter) {
	weights := make([]float64, len(vmIDs))
	totalWeight := 1.0 // Leave some system usage unattributed to VMs.
	for index := range weights {
		weights[index] = 0.1 + rand.Float64()
		totalWeight += weights[index]
	}
	for index, vmID := range vmIDs {
		_, _ = fmt.Fprintf(w, "vm_attribution_coefficient{name=\"%s\",resource_consumer_id=\"/qemu.slice/%d.scope\"} %.6f\n", hostName, vmID, weights[index]/totalWeight)
	}
}

// Simulate a basic Prometheus exporter with per-VM system usage attribution.
func metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(vmAttributionBase))
	simulateVMAttribution(w)
}

func main() {
	http.HandleFunc("/metrics", metrics)

	log.Println("Starting mock Prometheus exporter.")
	log.Println("\tHostname: " + hostName)
	log.Println("\tVM IDs: " + fmt.Sprint(vmIDs))
	log.Println("Server listening on 0.0.0.0:" + port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}
