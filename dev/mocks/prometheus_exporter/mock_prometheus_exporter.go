package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"chantico/dev/mocks/simulation"
)

const vmAttributionBase = `# HELP vm_attribution_coefficient Fraction of total system usage attributed to this VM.
# TYPE vm_attribution_coefficient gauge
`

var (
	hostName = simulation.EnvString("MOCK_PROMETHEUS_HOST_NAME", "prometheus-mock-bm-1")
	vmIDs    = simulation.EnvInts("MOCK_PROMETHEUS_VM_IDS", []int{120, 121})
	port     = simulation.EnvString("MOCK_PROMETHEUS_PORT", "9090")

	weights = simulation.NewWalk(simulation.BoundsFromEnv("MOCK_PROMETHEUS_WEIGHT", simulation.Bounds{
		Min:      0.2,
		Max:      1.0,
		Start:    0.6,
		Variance: 0.02,
	}))
)

func simulateVMAttribution(w http.ResponseWriter) {
	vmWeights := make([]float64, len(vmIDs))
	totalWeight := 1.0 // Leave some system usage unattributed to VMs.
	for index, vmID := range vmIDs {
		vmWeights[index] = weights.Next(strconv.Itoa(vmID))
		totalWeight += vmWeights[index]
	}
	for index, vmID := range vmIDs {
		_, _ = fmt.Fprintf(w, "vm_attribution_coefficient{name=\"%s\",resource_consumer_id=\"/qemu.slice/%d.scope\"} %.6f\n", hostName, vmID, vmWeights[index]/totalWeight)
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
