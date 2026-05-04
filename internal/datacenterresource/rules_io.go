/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datacenterresource

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	vol "chantico/internal/volumes"

	"go.yaml.in/yaml/v2"
)

const prometheusRulesDir = "prometheus/rules"

// reloadPrometheus sends a POST to the Prometheus /-/reload endpoint so that
// newly written (or deleted) rule files are picked up.  Requires Prometheus to
// be started with --web.enable-lifecycle.
func reloadPrometheus() {
	host := os.Getenv("CHANTICO_PROMETHEUS_SERVICE_HOST")
	port := os.Getenv("CHANTICO_PROMETHEUS_SERVICE_PORT")
	if host == "" || port == "" {
		log.Println("Prometheus host/port not configured, skipping reload")
		return
	}
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

// WriteRuleFile writes a Prometheus recording rule file for this DataCenterResource.
// The file is written to prometheus/rules/<name>.yml on the shared volume.
// After writing, Prometheus is sent a reload request so it picks up the new rules.
func WriteRuleFile(
	ctx context.Context,
	dataCenterResource *chantico.DataCenterResource,
) *sm.ActionResult {
	ruleFile := BuildRuleFile(dataCenterResource)

	// If there are no rules to write (e.g. root node with no children),
	// clean up any stale rule file and return.
	if ruleFile == nil {
		deleteRuleFileFromDisk(dataCenterResource.Name)
		return nil
	}

	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		log.Printf("Failed to create rules directory: %v", err)
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		log.Printf("Failed to marshal rule file: %v", err)
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	rulePath := filepath.Join(rulesDir, dataCenterResource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		log.Printf("Failed to write rule file: %v", err)
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	log.Printf("Wrote recording rule file %s for resource %s\n", rulePath, dataCenterResource.Name)
	reloadPrometheus()
	return nil
}

// DeleteRuleFile removes the Prometheus recording rule file for this DataCenterResource.
// After deleting, Prometheus is sent a reload request so it stops evaluating the removed rules.
func DeleteRuleFile(
	ctx context.Context,
	dataCenterResource *chantico.DataCenterResource,
) *sm.ActionResult {
	deleteRuleFileFromDisk(dataCenterResource.Name)
	reloadPrometheus()
	return nil
}

// deleteRuleFileFromDisk removes the rule file for the named resource.
func deleteRuleFileFromDisk(resourceName string) {
	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	rulePath := filepath.Join(volumePath, prometheusRulesDir, resourceName+".yml")

	log.Printf("Deleting rule file for %s\n", resourceName)

	err := os.Remove(rulePath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to delete rule file %s: %v", rulePath, err)
	}
}
