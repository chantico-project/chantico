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

	"net/http"
	"os"
	"path/filepath"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"

	"go.yaml.in/yaml/v2"
	log "sigs.k8s.io/controller-runtime/pkg/log"
)

const prometheusRulesDir = "prometheus/rules"

// reloadPrometheus sends a POST to the Prometheus /-/reload endpoint so that
// newly written (or deleted) rule files are picked up.  Requires Prometheus to
// be started with --web.enable-lifecycle.
func ReloadPrometheus(ctx context.Context) {
	l := log.FromContext(ctx)
	host := config.ValidatedEnv.PrometheusServiceHost
	port := config.ValidatedEnv.PrometheusServicePort
	url := fmt.Sprintf("http://%s:%s/-/reload", host, port)
	resp, err := http.Post(url, "", nil)
	if err != nil {
		l.Error(err, "Failed to reload Prometheus")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		l.Info("Prometheus reload returned status", "status", resp.StatusCode)
		return
	}
	l.Info("Prometheus configuration reloaded")
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
		DeleteRuleFileFromDisk(dataCenterResource.Name)
		return nil
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		l := log.FromContext(ctx)
		l.Error(err, "Failed to create rules directory")
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		l := log.FromContext(ctx)
		l.Error(err, "Failed to marshal rule file")
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	rulePath := filepath.Join(rulesDir, dataCenterResource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		l := log.FromContext(ctx)
		l.Error(err, "Failed to write rule file")
		SetValidationError(dataCenterResource, err, "")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	l := log.FromContext(ctx)
	l.Info("Wrote recording rule file", "file", rulePath, "resource", dataCenterResource.Name)
	ReloadPrometheus(ctx)
	return nil
}

// DeleteRuleFile removes the Prometheus recording rule file for this DataCenterResource.
// After deleting, Prometheus is sent a reload request so it stops evaluating the removed rules.
func DeleteRuleFile(
	ctx context.Context,
	dataCenterResource *chantico.DataCenterResource,
) *sm.ActionResult {
	DeleteRuleFileFromDisk(dataCenterResource.Name)
	ReloadPrometheus(ctx)
	return nil
}

// DeleteRuleFileFromDisk removes the rule file for the named resource.
func DeleteRuleFileFromDisk(resourceName string) {
	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, resourceName+".yml")

	l := log.Log.WithName("rules_io")
	l.Info("Deleting rule file", "resource", resourceName)

	err := os.Remove(rulePath)
	if err != nil && !os.IsNotExist(err) {
		l := log.Log.WithName("rules_io")
		l.Error(err, "Failed to delete rule file", "file", rulePath)
	}
}
