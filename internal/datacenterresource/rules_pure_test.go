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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExpectedRule struct {
	Record         string
	Expr           string
	ServiceIdLabel string
}

func testExpectedRule(t *testing.T, rule RecordingRule, expected ExpectedRule) {
	if rule.Record != expected.Record {
		t.Errorf("Expected rule record = %q, got %q", expected.Record, rule.Record)
	}
	if rule.Expr != expected.Expr {
		t.Errorf("Expected rule expr = %q, got %q", expected.Expr, rule.Expr)
	}
	if expected.ServiceIdLabel != "" {
		if val, ok := rule.Labels["serviceId"]; !ok || val != expected.ServiceIdLabel {
			t.Errorf("Expected serviceId label = %q, got %q", expected.ServiceIdLabel, val)
		}
	}
}

func TestSanitizeMetricName(t *testing.T) {
	testCases := map[string]struct {
		input    string
		expected string
	}{
		"simple name": {
			input:    "pdu1",
			expected: "pdu1",
		},
		"name with hyphens": {
			input:    "datacenterresource-pdu1",
			expected: "datacenterresource_pdu1",
		},
		"name with dots": {
			input:    "my.resource.name",
			expected: "my_resource_name",
		},
		"mixed special chars": {
			input:    "pdu-1.rack/2",
			expected: "pdu_1_rack_2",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := sanitizeMetricName(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeMetricName(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestEnergyMetricName(t *testing.T) {
	testCases := map[string]struct {
		resourceName string
		expected     string
	}{
		"simple": {
			resourceName: "bm1",
			expected:     "datacenter:bm1:energy_watts",
		},
		"with hyphens": {
			resourceName: "datacenterresource-misd-gbm-01",
			expected:     "datacenter:datacenterresource_misd_gbm_01:energy_watts",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := EnergyMetricName(tc.resourceName)
			if result != tc.expected {
				t.Errorf("EnergyMetricName(%q) = %q, want %q", tc.resourceName, result, tc.expected)
			}
		})
	}
}

func TestCoefficientMetricName(t *testing.T) {
	testCases := map[string]struct {
		parentName string
		childName  string
		expected   string
	}{
		"simple": {
			parentName: "bm1",
			childName:  "vm1",
			expected:   "coefficient_bm1_vm1",
		},
		"with hyphens": {
			parentName: "datacenterresource-bm1",
			childName:  "datacenterresource-vm1",
			expected:   "coefficient_datacenterresource_bm1_datacenterresource_vm1",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := CoefficientMetricName(tc.parentName, tc.childName)
			if result != tc.expected {
				t.Errorf("CoefficientMetricName(%q, %q) = %q, want %q",
					tc.parentName, tc.childName, result, tc.expected)
			}
		})
	}
}

func TestBuildRecordingRules_RootNodeNoChildren(t *testing.T) {
	// Root node with energyMetric produces 1 alias rule
	pdu := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
			ServiceId:    "3d88f471-674f-4446-9de2-54e5faa2c951",
		},
	}

	rules := BuildRecordingRules(pdu)
	if len(rules) != 1 {
		t.Fatalf("Expected 1 alias rule for root node, got %d rules", len(rules))
	}
	testExpectedRule(t, rules[0], ExpectedRule{
		Record:         "datacenter:pdu1:energy_watts",
		Expr:           "snmp_pdu1_power_watts",
		ServiceIdLabel: "3d88f471-674f-4446-9de2-54e5faa2c951",
	})
}

func TestBuildRecordingRules_RootNodeWithParentsWithCoefficients(t *testing.T) {
	// Non-root node with parents that have coefficients should produce
	// coefficient rules + energy rule.
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.5"},
				{Name: "pdu2", Coefficient: "0.5"},
			},
		},
	}

	rules := BuildRecordingRules(bm)
	// 2 coefficient rules + 1 energy rule = 3
	if len(rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(rules))
	}

	// Coefficient rules should be sorted by parent name
	testExpectedRule(t, rules[0], ExpectedRule{
		Record: "coefficient_pdu1_bm1",
		Expr:   "0.5",
	})
	testExpectedRule(t, rules[1], ExpectedRule{
		Record: "coefficient_pdu2_bm1",
		Expr:   "0.5",
	})
}

func TestBuildRecordingRules_NonRootWithParentsAndChildren(t *testing.T) {
	// BM node with 2 PDU parents (with coefficients) — no children defined here
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.3"},
				{Name: "pdu2", Coefficient: "0.7"},
			},
		},
	}

	rules := BuildRecordingRules(bm)
	// 2 coefficient rules + 1 energy rule = 3
	if len(rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(rules))
	}

	// First two should be coefficient rules (sorted by parent name)
	testExpectedRule(t, rules[0], ExpectedRule{
		Record: "coefficient_pdu1_bm1",
		Expr:   "0.3",
	})
	testExpectedRule(t, rules[1], ExpectedRule{
		Record: "coefficient_pdu2_bm1",
		Expr:   "0.7",
	})

	// Last should be the energy rule
	testExpectedRule(t, rules[2], ExpectedRule{
		Record: "datacenter:bm1:energy_watts",
		Expr:   "coefficient_pdu1_bm1 * on() datacenter:pdu1:energy_watts + coefficient_pdu2_bm1 * on() datacenter:pdu2:energy_watts",
	})
}

func TestBuildRecordingRules_LeafNode(t *testing.T) {
	// VM (leaf) with one parent and no children
	vm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:      DataCenterResourceTypeVM,
			ServiceId: "a479357a-2680-4577-8ffe-5105e634c836",
			Parents:   []chantico.ParentRef{{Name: "bm1"}},
		},
	}

	rules := BuildRecordingRules(vm)
	// Only 1 energy rule (no coefficient rules, no children)
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	testExpectedRule(t, rules[0], ExpectedRule{
		Record:         "datacenter:vm1:energy_watts",
		Expr:           "coefficient_bm1_vm1 * on() datacenter:bm1:energy_watts",
		ServiceIdLabel: "a479357a-2680-4577-8ffe-5105e634c836",
	})
}

func TestBuildRuleFile_RootNoChildren(t *testing.T) {
	pdu := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
		},
	}

	ruleFile := BuildRuleFile(pdu)
	if ruleFile == nil {
		t.Fatal("Expected non-nil rule file for root node with energyMetric")
	}
	if len(ruleFile.Groups[0].Rules) != 1 {
		t.Errorf("Expected 1 alias rule, got %d", len(ruleFile.Groups[0].Rules))
	}
}

func TestBuildRuleFile_WithRules(t *testing.T) {
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.6"},
			},
		},
	}

	ruleFile := BuildRuleFile(bm)
	if ruleFile == nil {
		t.Fatal("Expected non-nil rule file")
	}
	if len(ruleFile.Groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(ruleFile.Groups))
	}
	if ruleFile.Groups[0].Name != "chantico_bm1" {
		t.Errorf("Expected group name %q, got %q", "chantico_bm1", ruleFile.Groups[0].Name)
	}
	// 1 coefficient rule + 1 energy rule = 2
	if len(ruleFile.Groups[0].Rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(ruleFile.Groups[0].Rules))
	}
}

func TestBuildRecordingRules_ThreeLayerHierarchy(t *testing.T) {
	// Complete three-layer example: PDU → BM → VM
	// PDU1 is a root node with no parents — no rules generated
	pdu1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1a"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1a_power_watts",
		},
	}
	pdu1Rules := BuildRecordingRules(pdu1)
	if len(pdu1Rules) != 1 {
		t.Fatalf("PDU1: expected 1 alias rule, got %d", len(pdu1Rules))
	}
	testExpectedRule(t, pdu1Rules[0], ExpectedRule{
		Record: "datacenter:pdu1a:energy_watts",
		Expr:   "snmp_pdu1a_power_watts",
	})

	// BM1 with parent PDU1 (coefficient "1"), generates coefficient + energy
	bm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1a", Coefficient: "1"},
			},
		},
	}
	bm1Rules := BuildRecordingRules(bm1)
	// 1 coefficient rule + 1 energy rule = 2
	if len(bm1Rules) != 2 {
		t.Fatalf("BM1: expected 2 rules, got %d", len(bm1Rules))
	}
	testExpectedRule(t, bm1Rules[0], ExpectedRule{
		Record: "coefficient_pdu1a_bm1",
		Expr:   "1",
	})

	// VM1 with parent BM1 (coefficient "0.4")
	vm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.4"},
			},
		},
	}
	vm1Rules := BuildRecordingRules(vm1)
	// 1 coefficient + 1 energy = 2
	if len(vm1Rules) != 2 {
		t.Fatalf("VM1: expected 2 rules, got %d", len(vm1Rules))
	}
	expectedExpr := "coefficient_bm1_vm1 * on() datacenter:bm1:energy_watts"
	if vm1Rules[1].Expr != expectedExpr {
		t.Errorf("VM1: expected expr %q, got %q", expectedExpr, vm1Rules[1].Expr)
	}

	// VM2 with parent BM1 (coefficient "0.6")
	vm2 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm2"},
		Spec: chantico.DataCenterResourceSpec{
			Type:      DataCenterResourceTypeVM,
			ServiceId: "002793bc-e100-4953-ac07-a25d12b573d4",
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.6"},
			},
		},
	}
	vm2Rules := BuildRecordingRules(vm2)
	// 1 coefficient + 1 energy = 2
	if len(vm2Rules) != 2 {
		t.Fatalf("VM2: expected 2 rules, got %d", len(vm2Rules))
	}
	expectedExpr = "coefficient_bm1_vm2 * on() datacenter:bm1:energy_watts"
	if vm2Rules[1].Expr != expectedExpr {
		t.Errorf("VM2: expected expr %q, got %q", expectedExpr, vm2Rules[1].Expr)
	}
}

func TestBuildRecordingRules_ManyToOneParents(t *testing.T) {
	// BM with two PDU parents (many-to-one)
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:    DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{{Name: "pdu2"}, {Name: "pdu1"}}, // intentionally unsorted
		},
	}

	rules := BuildRecordingRules(bm)
	// Only 1 energy rule (no children)
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	// Parents should be sorted in the expression
	expectedExpr := "coefficient_pdu1_bm1 * on() datacenter:pdu1:energy_watts + coefficient_pdu2_bm1 * on() datacenter:pdu2:energy_watts"
	if rules[0].Expr != expectedExpr {
		t.Errorf("Expected expr %q, got %q", expectedExpr, rules[0].Expr)
	}
}
