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
	"fmt"
	"sort"
	"strings"

	chantico "chantico/api/v1alpha1"
)

// RecordingRule represents a single Prometheus recording rule.
type RecordingRule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}

// RuleGroup represents a Prometheus rule group.
type RuleGroup struct {
	Name  string          `yaml:"name"`
	Rules []RecordingRule `yaml:"rules"`
}

// RuleFile represents a complete Prometheus rule file.
type RuleFile struct {
	Groups []RuleGroup `yaml:"groups"`
}

// EnergyMetricName returns the deterministic Prometheus metric name for a
// DataCenterResource's energy timeseries.
func EnergyMetricName(resourceName string) string {
	sanitized := sanitizeMetricName(resourceName)
	return fmt.Sprintf("datacenter:%s:energy_watts", sanitized)
}

// CoefficientMetricName returns the deterministic Prometheus metric name for
// the coefficient from parent to child.
func CoefficientMetricName(parentName, childName string) string {
	sanitizedParent := sanitizeMetricName(parentName)
	sanitizedChild := sanitizeMetricName(childName)
	return fmt.Sprintf("coefficient_%s_%s", sanitizedParent, sanitizedChild)
}

// sanitizeMetricName replaces characters that are not valid in Prometheus
// metric names with underscores.
func sanitizeMetricName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)
}

// BuildRecordingRules generates the set of Prometheus recording rules for a
// DataCenterResource node, following the energy accounting design:
//
//  1. For root nodes (spec.energyMetric is set), an alias rule mapping the raw
//     energy metric to the canonical datacenter:<name>:energy_watts name.
//  2. One coefficient recording rule per parent that has a coefficient set
//     (from the ParentRef entries in spec.parents).
//  3. One energy recording rule for non-root nodes (sum of coefficient * parent
//     energy for each parent).
//
// Returns nil if no rules need to be written.
func BuildRecordingRules(
	dataCenterResource *chantico.DataCenterResource,
) []RecordingRule {
	var rules []RecordingRule

	// 1. Root-node alias: map raw energyMetric → canonical name
	if aliasRule := buildEnergyAliasRule(dataCenterResource); aliasRule != nil {
		rules = append(rules, *aliasRule)
	}

	// 2. Coefficient rules for each parent
	rules = append(rules, buildCoefficientRules(dataCenterResource)...)

	// 3. Energy rule for this node (non-root only)
	energyRule := buildEnergyRule(dataCenterResource)
	if energyRule != nil {
		rules = append(rules, *energyRule)
	}

	return rules
}

// buildEnergyAliasRule creates a recording rule for root nodes that aliases
// the raw energy metric (e.g. tnoPduPowerValue{instance="..."}) to the
// canonical datacenter:<name>:energy_watts name. This allows children to
// reference the parent's energy using a uniform naming convention.
//
// Returns nil if it is not a root node (has parents).
func buildEnergyAliasRule(
	dataCenterResource *chantico.DataCenterResource,
) *RecordingRule {
	if len(dataCenterResource.Spec.Parents) > 0 {
		return nil
	}
	return &RecordingRule{
		Record: EnergyMetricName(dataCenterResource.Name),
		Expr:   dataCenterResource.Spec.EnergyMetric,
	}
}

// buildCoefficientRules creates one recording rule per parent that has a
// coefficient set. The coefficient is defined on the child's ParentRef and
// represents the proportional share of the parent's energy attributable to
// this child.
func buildCoefficientRules(
	dataCenterResource *chantico.DataCenterResource,
) []RecordingRule {
	if len(dataCenterResource.Spec.Parents) == 0 {
		return nil
	}

	// Collect parents that have a coefficient set, sorted by parent name
	// for deterministic output.
	type parentCoeff struct {
		parentName string
		coeff      string
	}
	var pcs []parentCoeff
	for _, p := range dataCenterResource.Spec.Parents {
		if p.Coefficient != "" {
			pcs = append(pcs, parentCoeff{parentName: p.Name, coeff: p.Coefficient})
		}
	}
	sort.Slice(pcs, func(i, j int) bool { return pcs[i].parentName < pcs[j].parentName })

	rules := make([]RecordingRule, 0, len(pcs))
	for _, pc := range pcs {
		rules = append(rules, RecordingRule{
			Record: CoefficientMetricName(pc.parentName, dataCenterResource.Name),
			Expr:   pc.coeff,
		})
	}
	return rules
}

// buildEnergyRule creates the energy recording rule for a non-root node.
// The rule computes the node's energy as a weighted sum of its parents' energy
// timeseries, using the coefficient timeseries written by those parents.
//
// For root nodes (no parents), returns nil — the energy timeseries is
// already present in Prometheus (e.g. from an SNMP exporter).
func buildEnergyRule(
	dataCenterResource *chantico.DataCenterResource,
) *RecordingRule {
	if len(dataCenterResource.Spec.Parents) == 0 {
		return nil
	}

	// Build the PromQL expression: sum of coefficient * parent_energy for each parent
	// Sort parents for deterministic output
	parentNames := dataCenterResource.Spec.ParentNames()
	sort.Strings(parentNames)

	terms := make([]string, 0, len(parentNames))
	for _, parentName := range parentNames {
		coeffMetric := CoefficientMetricName(parentName, dataCenterResource.Name)
		parentEnergyMetric := EnergyMetricName(parentName)
		terms = append(terms, fmt.Sprintf("%s * on() %s", coeffMetric, parentEnergyMetric))
	}

	return &RecordingRule{
		Record: EnergyMetricName(dataCenterResource.Name),
		Expr:   strings.Join(terms, " + "),
	}
}

// BuildRuleFile wraps the recording rules into a complete Prometheus rule file
// structure with a single group named after the resource.
func BuildRuleFile(
	dataCenterResource *chantico.DataCenterResource,
) *RuleFile {
	rules := BuildRecordingRules(dataCenterResource)
	if len(rules) == 0 {
		return nil
	}

	return &RuleFile{
		Groups: []RuleGroup{
			{
				Name:  fmt.Sprintf("chantico_%s", sanitizeMetricName(dataCenterResource.Name)),
				Rules: rules,
			},
		},
	}
}
