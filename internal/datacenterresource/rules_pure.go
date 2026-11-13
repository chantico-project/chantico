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
	"fmt"
	"maps"
	"sort"
	"strings"
)

type labels = map[string]string

// RecordingRule represents a single Prometheus recording rule.
type RecordingRule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
	Labels labels `yaml:"labels,omitempty"`
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

type RuleBuilder struct {
	resource    *chantico.DataCenterResource
	expressions *ResolvedExpressions
}

func NewRuleBuilder(resource *chantico.DataCenterResource, expressions *ResolvedExpressions) *RuleBuilder {
	return &RuleBuilder{resource: resource, expressions: expressions}
}

// CoefficientMetricName is the Prometheus metric name for energy coefficients in the case where
// the device has parent(s) and the energy is a factor of the parent's energy.
const CoefficientMetricName = "chantico_power_coefficient"

// EnergyMetricName is the Prometheus metric name for the energy timeseries of a DataCenterResource.
// This metric comes from the rules generated in this file.
const EnergyMetricName = "chantico_power_watts"

const (
	EnergyMetricKindAttributed = "attributed"
	EnergyMetricKindOverhead   = "overhead"
	EnergyMetricKindTotal      = "total"
)

const (
	EnergyCoefficientKindAttribution = "attribution"
	EnergyCoefficientKindOverhead    = "overhead"
	// No total is needed since sum(attributed) + overhead = 1 always!
)

// EnergyMetricQuery returns the Prometheus query for a DataCenterResource's energy timeseries.
func EnergyMetricQuery(resourceName string) string {
	return fmt.Sprintf(`%s{resource="%s"}`, EnergyMetricName, resourceName)
}

// coefficientMetricQuery returns the Prometheus query for the coefficient from parent to child.
func coefficientMetricQuery(parentName, childName string) string {
	return fmt.Sprintf(`%s{parent="%s", child="%s"}`, CoefficientMetricName, parentName, childName)
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
//     energy metric to a labeled series under the `chantico_power_watts` name.
//  2. One coefficient recording rule per parent that has a coefficient set
//     (from the ParentRef entries in spec.parents).
//  3. One energy recording rule for non-root nodes (sum of coefficient * parent
//     energy for each parent). Uses the same `chantico_power_watts` name.
//
// Returns nil if no rules need to be written.
func (b *RuleBuilder) BuildRecordingRules() []RecordingRule {
	var rules []RecordingRule

	for _, builder := range ruleBuilders {
		rules = append(rules, builder(b)...)
	}

	return rules
}

func applyAdditionalLabels(base map[string]string, labels map[string]string) map[string]string {
	// Prefer the base labels over the additional labels
	merged := maps.Clone(labels)
	maps.Copy(merged, base)
	return merged
}

func (b *RuleBuilder) constructParentNamesLabel() string {
	parents := b.resource.Spec.ParentNames()
	return strings.Join(parents, ",")
}

// buildSharedLabels builds the set of labels for the prometheus rule.
// It includes the resource name, type, service ID, parent names, any base labels, and additional labels specified in the resource.
func (b *RuleBuilder) buildSharedLabels(base labels) labels {
	labels := labels{
		"resource": b.resource.Name,
		"type":     b.resource.Spec.Type,
	}

	if len(b.resource.Spec.Parents) > 0 {
		labels["parents"] = b.constructParentNamesLabel()
	}

	if base != nil {
		labels = applyAdditionalLabels(labels, base)
	}

	if b.resource.Spec.AdditionalLabels != nil {
		labels = applyAdditionalLabels(labels, b.resource.Spec.AdditionalLabels)
	}

	return labels
}

// buildEnergyAliasRule creates a recording rule for root nodes that aliases
// the raw energy metric (e.g. tnoPduPowerValue{instance="..."}) to the
// a labelled time series under the `chantico_power_watts` name. This allows
// children to reference the parent's energy using a uniform naming convention.
//
// Returns nil if it is not a root node (has parents).
func (b *RuleBuilder) buildEnergyAliasRule() []RecordingRule {
	if len(b.resource.Spec.Parents) > 0 {
		return nil
	}

	// Construct the initial set of labels for the recording rule.
	baseLabels := map[string]string{
		"kind": EnergyMetricKindTotal,
	}

	if len(b.resource.Spec.Parents) > 0 {
		baseLabels["parents"] = b.constructParentNamesLabel()
	}

	// Add the defaults from the shared labels based on the resource spec.
	labels := b.buildSharedLabels(baseLabels)

	return []RecordingRule{
		{
			Record: EnergyMetricName,
			Expr:   b.expressions.EnergyMetric,
			Labels: labels,
		},
	}
}

// buildCoefficientRules creates one recording rule per parent that has a
// coefficient set. The coefficient is defined on the child's ParentRef and
// represents the proportional share of the parent's energy attributable to
// this child.
func (b *RuleBuilder) buildCoefficientRules() []RecordingRule {
	if len(b.expressions.Parents) == 0 {
		return nil
	}

	// Collect parents that have a coefficient set, sorted by parent name
	// for deterministic output.
	type parentCoeff struct {
		parentName string
		coeff      string
	}
	var pcs []parentCoeff
	for _, p := range b.expressions.Parents {
		if p.Coefficient != "" {
			pcs = append(pcs, parentCoeff{parentName: p.Name, coeff: p.Coefficient})
		}
	}
	sort.Slice(pcs, func(i, j int) bool { return pcs[i].parentName < pcs[j].parentName })

	baseLabels := map[string]string{
		"child": b.resource.Name,
		"kind":  EnergyCoefficientKindAttribution,
	}

	rules := make([]RecordingRule, 0, len(pcs))
	for _, pc := range pcs {
		baseLabels["parent"] = pc.parentName
		labels := b.buildSharedLabels(baseLabels)

		rules = append(rules, RecordingRule{
			Record: CoefficientMetricName,
			Expr:   pc.coeff,
			Labels: labels,
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
func (b *RuleBuilder) buildEnergyRule() []RecordingRule {
	if len(b.resource.Spec.Parents) == 0 {
		return nil
	}

	// Construct the initial set of labels for the recording rule.
	labels := map[string]string{
		"kind": EnergyMetricKindTotal,
	}
	// Merge the shared labels into the initial set of labels.
	maps.Copy(labels, b.buildSharedLabels(labels))

	// Construct the Prometheus aggregation expression for the energy recording rule.
	expr := fmt.Sprintf(
		`sum(%s{child="%s"} * on (parent) group_left () label_replace(%s{kind="%s"}, "parent", "$1", "resource", "(.*)"))`,
		CoefficientMetricName, b.resource.Name, EnergyMetricName, EnergyMetricKindTotal,
	)

	rule := RecordingRule{
		Record: EnergyMetricName,
		Expr:   expr,
		Labels: labels,
	}

	return []RecordingRule{rule}
}

var ruleBuilders = []func(*RuleBuilder) []RecordingRule{
	// 1. Root-node alias: map raw energyMetric → canonical name
	(*RuleBuilder).buildEnergyAliasRule,
	// 2. Coefficient rules for each parent
	(*RuleBuilder).buildCoefficientRules,
	// 3. Energy rule for this node (non-root only)
	(*RuleBuilder).buildEnergyRule,
}

// buildRecordingRules generates the set of Prometheus recording rules for a
// DataCenterResource node, following the energy accounting design:
//
//  1. For root nodes (spec.energyMetric is set), an alias rule mapping the raw
//     energy metric to a labeled series under the `chantico_power_watts` name.
//  2. One coefficient recording rule per parent that has a coefficient set
//     (from the ParentRef entries in spec.parents).
//  3. One energy recording rule for non-root nodes (sum of coefficient * parent
//     energy for each parent). Uses the same `chantico_power_watts` name.
//
// Returns nil if no rules need to be written.
// BuildRuleFile wraps the recording rules into a complete Prometheus rule file
// structure with a single group named after the resource.
func (b *RuleBuilder) BuildRuleFile() *RuleFile {
	rules := b.BuildRecordingRules()
	if len(rules) == 0 {
		return nil
	}

	return &RuleFile{
		Groups: []RuleGroup{
			{
				Name:  fmt.Sprintf("chantico_%s", sanitizeMetricName(b.resource.Name)),
				Rules: rules,
			},
		},
	}
}
