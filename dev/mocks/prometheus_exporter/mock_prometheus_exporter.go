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

const raplMetricsBase = `# HELP rapl_consumed_energy_joules Energy consumed since the previous measurement, as reported by RAPL.
# TYPE rapl_consumed_energy_joules gauge
# UNIT rapl_consumed_energy_joules joules
`

const vmEnergyAttributionBase = ` # HELP attributed_energy_joules Energy attribution (since the previous value) per consumer and per resource.
# TYPE attributed_energy_joules gauge
# UNIT attributed_energy_joules joules
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

func simulateRaplMetrics(w http.ResponseWriter, r *http.Request) {
	// Simulate energy consumption values in a realistic range for demonstration purposes
	packageEnergy := 60 + rand.Float64()*60
	dramEnergy := 8 + rand.Float64()*16

	_, _ = fmt.Fprintf(w, "rapl_consumed_energy_joules{domain=\"dram_total\",name=\"%s\",resource_consumer_id=\"\",resource_consumer_kind=\"local_machine\",resource_id=\"\",resource_kind=\"local_machine\"} %.6f\n", hostName, dramEnergy)
	_, _ = fmt.Fprintf(w, "rapl_consumed_energy_joules{domain=\"package_total\",name=\"%s\",resource_consumer_id=\"\",resource_consumer_kind=\"local_machine\",resource_id=\"\",resource_kind=\"local_machine\"} %.6f\n", hostName, packageEnergy)
}

func simulateVMEnergyAttribution(w http.ResponseWriter, r *http.Request) {
	// Simulate energy attribution values for demonstration purposes
	_, _ = fmt.Fprintf(w, vmEnergyAttributionBase)

	sumEnergy := 0.0

	for _, vmID := range vmIDs {
		vmEnergy := 0.01 + rand.Float64()*0.05 // Simulate energy attribution for each VM
		sumEnergy += vmEnergy

		_, _ = fmt.Fprintf(w, "attributed_energy_joules{domain=\"package_total\",kind=\"total\",resource_consumer_id=\"/qemu.slice/%d.scope\",resource_consumer_kind=\"cgroup\",resource_id=\"\",resource_kind=\"local_machine\"} %.6f\n", vmID, vmEnergy)
	}
	overheadQemu := 0.005 + rand.Float64()*0.01 // Simulate energy attribution for QEMU overhead
	sumEnergy += overheadQemu
	_, _ = fmt.Fprintf(w, "attributed_energy_joules{domain=\"package_total\",kind=\"total\",resource_consumer_id=\"/qemu.slice\",resource_consumer_kind=\"cgroup\",resource_id=\"\",resource_kind=\"local_machine\"} %.6f\n", sumEnergy)

}

// Simulate a very basic prometheus exporter that serves RAPL energy consumption metrics. The values are randomly generated for demonstration purposes.
func metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(raplMetricsBase))

	simulateRaplMetrics(w, r)
	simulateVMEnergyAttribution(w, r)
}

func main() {
	http.HandleFunc("/metrics", metrics)

	log.Println("Starting mock Prometheus exporter.")
	log.Println("\tHostname: " + hostName)
	log.Println("\tVM IDs: " + fmt.Sprint(vmIDs))
	log.Println("Server listening on 0.0.0.0:" + port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}
