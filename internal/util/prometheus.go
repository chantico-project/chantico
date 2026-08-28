package util

import (
	"fmt"
	"log"
	"net/http"

	config "chantico/internal/configuration"
)

// reloadPrometheus sends a POST to the Prometheus /-/reload endpoint so that
// newly written (or deleted) rule files are picked up.  Requires Prometheus to
// be started with --web.enable-lifecycle.
func ReloadPrometheus() {
	host := config.ValidatedEnv.PrometheusServiceHost
	port := config.ValidatedEnv.PrometheusServicePort
	url := fmt.Sprintf("http://%s:%s/-/reload", host, port)
	resp, err := http.Post(url, "", nil)
	if err != nil {
		log.Printf("Failed to reload Prometheus: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Prometheus reload returned status %d", resp.StatusCode)
		return
	}
	log.Println("Prometheus configuration reloaded")
}
