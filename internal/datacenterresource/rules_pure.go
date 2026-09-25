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
	"maps"
	"sort"
	"strings"

	chantico "chantico/api/v1alpha1"
)

// RecordingRule represents a single Prometheus recording rule.
type RecordingRule struct {
	Record string            `yaml:"record"`
	Expr   string            `yaml:"expr"`
	Labels map[string]string `yaml:"labels,omitempty"`
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

// CoefficientMetricName is the Prometheus metric name for energy coefficients in the case where
// the device has parent(s) and the energy is a factor of the parent's energy.
const CoefficientMetricName = "chantico_energy_coefficient"

// EnergyMetricName is the Prometheus metric name for the energy timeseries of a DataCenterResource.
// This metric comes from the rules generated in this file.
const EnergyMetricName = "chantico_energy_watts"

// EnergyMetricQuery returns the Prometheus query for a DataCenterResource's energy timeseries.
func EnergyMetricQuery(resourceName string) string {
	return fmt.Sprintf(`%s{resource="%s"}`, EnergyMetricName, resourceName)
}

// CoefficientMetricQuery returns the Prometheus query for the coefficient from parent to child.
func CoefficientMetricQuery(parentName, childName string) string {
	return fmt.Sprintf(`%s{parent="%s", child="%s"}`, CoefficientMetricName, parentName, childName)
}

// SanitizeMetricName replaces characters that are not valid in Prometheus
// metric names with underscores.
func SanitizeMetricName(name string) string {
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
//     energy metric to a labeled series under the `chantico_energy_watts` name.
//  2. One coefficient recording rule per parent that has a coefficient set
//     (from the ParentRef entries in spec.parents).
//  3. One energy recording rule for non-root nodes (sum of coefficient * parent
//     energy for each parent). Uses the same `chantico_energy_watts` name.
//
// Returns nil if no rules need to be written.
func BuildRecordingRules(
	dataCenterResource *chantico.DataCenterResource,
) []RecordingRule {
	var rules []RecordingRule

	// 1. Root-node alias: map raw energyMetric → canonical name
	if aliasRule := BuildEnergyAliasRule(dataCenterResource); aliasRule != nil {
		rules = append(rules, *aliasRule)
	}

	// 2. Coefficient rules for each parent
	rules = append(rules, BuildCoefficientRules(dataCenterResource)...)

	// 3. Energy rule for this node (non-root only)
	energyRule := BuildEnergyRule(dataCenterResource)
	if energyRule != nil {
		rules = append(rules, *energyRule)
	}

	return rules
}

func applyAdditionalLabels(base map[string]string, labels map[string]string) map[string]string {
	for k, v := range labels {
		if _, exists := base[k]; !exists {
			base[k] = v
		}
	}
	return base
}

func constructParentNamesLabel(dataCenterResource *chantico.DataCenterResource) string {
	parents := []string{}
	for _, name := range dataCenterResource.Spec.ParentNames() {
		parents = append(parents, name)
	}
	return strings.Join(parents, ",")
}

// buildSharedLabels builds the set of labels for the prometheus rule.
// It includes the resource name, type, service ID, parent names, any base labels, and additional labels specified in the resource.
func buildSharedLabels(dataCenterResource *chantico.DataCenterResource, base map[string]string) map[string]string {
	labels := map[string]string{
		"resource": dataCenterResource.Name,
		"type":     dataCenterResource.Spec.Type,
	}

	if dataCenterResource.Spec.ServiceId != "" {
		labels["serviceId"] = dataCenterResource.Spec.ServiceId
	}

	if len(dataCenterResource.Spec.Parents) > 0 {
		labels["parents"] = constructParentNamesLabel(dataCenterResource)
	}

	if base != nil {
		labels = applyAdditionalLabels(labels, base)
	}

	if dataCenterResource.Spec.AdditionalLabels != nil {
		labels = applyAdditionalLabels(labels, dataCenterResource.Spec.AdditionalLabels)
	}

	return labels
}

// buildEnergyAliasRule creates a recording rule for root nodes that aliases
// the raw energy metric (e.g. tnoPduPowerValue{instance="..."}) to the
// a labelled time series under the `chantico_energy_watts` name. This allows
// children to reference the parent's energy using a uniform naming convention.
//
// Returns nil if it is not a root node (has parents).
func BuildEnergyAliasRule(
	dataCenterResource *chantico.DataCenterResource,
) *RecordingRule {
	if len(dataCenterResource.Spec.Parents) > 0 {
		return nil
	}

	// Construct the initial set of labels for the recording rule.
	baseLabels := map[string]string{
		// "customLabel": "exampleValue",
	}

	// Add the defaults from the shared labels based on the resouce spec.
	labels := buildSharedLabels(dataCenterResource, baseLabels)

	return &RecordingRule{
		Record: EnergyMetricName,
		Expr:   dataCenterResource.Spec.EnergyMetric,
		Labels: labels,
	}
}

// buildCoefficientRules creates one recording rule per parent that has a
// coefficient set. The coefficient is defined on the child's ParentRef and
// represents the proportional share of the parent's energy attributable to
// this child.
func BuildCoefficientRules(
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
			Record: CoefficientMetricName,
			Expr:   pc.coeff,
			Labels: map[string]string{
				"parent": pc.parentName,
				"child":  dataCenterResource.Name,
			},
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
func BuildEnergyRule(
	dataCenterResource *chantico.DataCenterResource,
) *RecordingRule {
	if len(dataCenterResource.Spec.Parents) == 0 {
		return nil
	}

	// Construct the initial set of labels for the recording rule.
	labels := map[string]string{
		// "customLabel": "exampleValue",
	}
	// Merge the shared labels into the initial set of labels.
	maps.Copy(labels, buildSharedLabels(dataCenterResource, labels))

	// Construct the Prometheus aggregation expression for the energy recording rule.
	expr := fmt.Sprintf(
		`sum(%s{child="%s"} * on (parent) group_left () label_replace(%s, "parent", "$1", "resource", "(.*)"))`,
		CoefficientMetricName, dataCenterResource.Name, EnergyMetricName,
	)

	return &RecordingRule{
		Record: EnergyMetricName,
		Expr:   expr,
		Labels: labels,
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
				Name:  fmt.Sprintf("chantico_%s", SanitizeMetricName(dataCenterResource.Name)),
				Rules: rules,
			},
		},
	}
}
