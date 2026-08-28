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
	"log"
	"os"
	"path/filepath"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	"chantico/internal/util"

	"go.yaml.in/yaml/v2"
)

const prometheusRulesDir = "prometheus/rules"

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

	volumePath := config.ValidatedEnv.VolumeLocation
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
	util.ReloadPrometheus()
	return nil
}

// DeleteRuleFile removes the Prometheus recording rule file for this DataCenterResource.
// After deleting, Prometheus is sent a reload request so it stops evaluating the removed rules.
func DeleteRuleFile(
	ctx context.Context,
	dataCenterResource *chantico.DataCenterResource,
) *sm.ActionResult {
	deleteRuleFileFromDisk(dataCenterResource.Name)
	util.ReloadPrometheus()
	return nil
}

// deleteRuleFileFromDisk removes the rule file for the named resource.
func deleteRuleFileFromDisk(resourceName string) {
	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, resourceName+".yml")

	log.Printf("Deleting rule file for %s\n", resourceName)

	err := os.Remove(rulePath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to delete rule file %s: %v", rulePath, err)
	}
}
