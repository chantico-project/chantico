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
	chantico "chantico/api/v1alpha1"
	vol "chantico/internal/volumes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testCreateRulesTmpDir(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv(vol.ChanticoVolumeLocationEnv, tmpDir)

	rulesDir := filepath.Join(tmpDir, prometheusRulesDir)
	err := os.MkdirAll(rulesDir, 0755)
	if err != nil {
		t.Fatalf("Could not create directory %s\n", rulesDir)
	}

	return tmpDir
}

func TestWriteRuleFile_NonRootNode(t *testing.T) {
	tmpDir := testCreateRulesTmpDir(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.6"},
			},
		},
	}

	result := WriteRuleFile(t.Context(), bm)
	if result != nil {
		t.Fatalf("Expected nil result, got %+v", result)
	}

	// Verify rule file was created
	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("Expected rule file to exist at %s: %v", rulePath, err)
	}

	// Parse and verify the content
	var ruleFile RuleFile
	if err := yaml.Unmarshal(data, &ruleFile); err != nil {
		t.Fatalf("Failed to parse rule file: %v", err)
	}

	if len(ruleFile.Groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(ruleFile.Groups))
	}
	if ruleFile.Groups[0].Name != "chantico_bm1" {
		t.Errorf("Expected group name %q, got %q", "chantico_bm1", ruleFile.Groups[0].Name)
	}
	// 1 coefficient rule + 1 energy rule = 2
	if len(ruleFile.Groups[0].Rules) != 2 {
		t.Fatalf("Expected 2 rules, got %d", len(ruleFile.Groups[0].Rules))
	}
}

func TestWriteRuleFile_RootNodeWithEnergyMetric(t *testing.T) {
	tmpDir := testCreateRulesTmpDir(t)

	pdu := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
		},
	}

	result := WriteRuleFile(t.Context(), pdu)
	if result != nil {
		t.Fatalf("Expected nil result, got %+v", result)
	}

	// Root with energyMetric should create a rule file with an alias rule
	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "pdu1.yml")
	if _, err := os.Stat(rulePath); err != nil {
		t.Fatalf("Expected rule file to exist at %s: %v", rulePath, err)
	}
}

func TestWriteRuleFile_OverwritesExisting(t *testing.T) {
	tmpDir := testCreateRulesTmpDir(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.5"},
			},
		},
	}

	// Write initial
	WriteRuleFile(t.Context(), bm)

	// Update coefficient and write again
	bm.Spec.Parents[0].Coefficient = "0.8"
	WriteRuleFile(t.Context(), bm)

	// Verify updated content
	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("Expected rule file to exist: %v", err)
	}

	if !strings.Contains(string(data), "0.8") {
		t.Errorf("Expected updated coefficient 0.8 in rule file, got:\n%s", string(data))
	}
}

func TestDeleteRuleFile(t *testing.T) {
	tmpDir := testCreateRulesTmpDir(t)

	// Create a rule file first
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.5"},
			},
		},
	}
	WriteRuleFile(t.Context(), bm)

	// Verify it exists
	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	if _, err := os.Stat(rulePath); err != nil {
		t.Fatalf("Expected rule file to exist before deletion: %v", err)
	}

	// Delete it
	result := DeleteRuleFile(t.Context(), bm)
	if result != nil {
		t.Fatalf("Expected nil result, got %+v", result)
	}

	// Verify it's gone
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Errorf("Expected rule file to be deleted, but it still exists")
	}
}

func TestDeleteRuleFile_NonExistent(t *testing.T) {
	testCreateRulesTmpDir(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		Spec:       chantico.DataCenterResourceSpec{},
	}

	// Should not panic or error on non-existent file
	result := DeleteRuleFile(t.Context(), bm)
	if result != nil {
		t.Fatalf("Expected nil result, got %+v", result)
	}
}

func TestWriteRuleFile_FullThreeLayerHierarchy(t *testing.T) {
	tmpDir := testCreateRulesTmpDir(t)

	// PDU1 → BM1 → VM1, VM2
	pdu1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
		},
	}

	bm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "1"},
			},
		},
	}

	vm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.4"},
			},
		},
	}

	vm2 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm2"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.6"},
			},
		},
	}

	// Write all rule files
	WriteRuleFile(t.Context(), pdu1)
	WriteRuleFile(t.Context(), bm1)
	WriteRuleFile(t.Context(), vm1)
	WriteRuleFile(t.Context(), vm2)

	rulesDir := filepath.Join(tmpDir, prometheusRulesDir)

	// PDU1 should have a rule file (alias rule for energyMetric)
	assertRuleFileExists(t, rulesDir, "pdu1.yml")

	// BM1 should have a rule file (has parent with coefficient)
	assertRuleFileExists(t, rulesDir, "bm1.yml")

	// VM1 should have a rule file (has parent with coefficient)
	assertRuleFileExists(t, rulesDir, "vm1.yml")

	// VM2 should have a rule file (has parent with coefficient)
	assertRuleFileExists(t, rulesDir, "vm2.yml")

	// Verify PDU1 rule file has the alias rule
	pdu1Data, _ := os.ReadFile(filepath.Join(rulesDir, "pdu1.yml"))
	var pdu1RuleFile RuleFile
	yaml.Unmarshal(pdu1Data, &pdu1RuleFile)

	if len(pdu1RuleFile.Groups[0].Rules) != 1 {
		t.Errorf("PDU1: expected 1 alias rule, got %d",
			len(pdu1RuleFile.Groups[0].Rules))
	}

	// Verify BM1 rule file has correct content
	bm1Data, _ := os.ReadFile(filepath.Join(rulesDir, "bm1.yml"))
	var bm1RuleFile RuleFile
	yaml.Unmarshal(bm1Data, &bm1RuleFile)

	if len(bm1RuleFile.Groups[0].Rules) != 2 {
		t.Errorf("BM1: expected 2 rules (1 coefficient + 1 energy), got %d",
			len(bm1RuleFile.Groups[0].Rules))
	}
}

func assertRuleFileExists(t *testing.T, rulesDir, filename string) {
	t.Helper()
	path := filepath.Join(rulesDir, filename)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected rule file %s to exist: %v", filename, err)
	}
}
