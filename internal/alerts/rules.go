/*
Copyright 2025-2026.

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

// Package alerts generates the Prometheus health alerts of Chantico resources
// and receives their state changes from Alertmanager to update conditions.
package alerts

import (
	"strconv"

	chantico "chantico/api/v1alpha1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Keep in sync with the AlertmanagerConfig groupBy in chart/templates/alertmanager.yaml.
const (
	LabelKind         = "chantico_kind"
	LabelName         = "chantico_name"
	LabelNamespace    = "namespace"
	LabelCondition    = "chantico_condition"
	AnnotationSummary = "summary"
)

const (
	KindPhysicalMeasurement = "PhysicalMeasurement"
	KindDataCenterResource  = "DataCenterResource"
)

// Rule describes a health alert of a resource. The alert sets Condition to False while it fires.
type Rule struct {
	Alert     string
	Expr      string
	For       monitoringv1.Duration
	Condition chantico.ConditionType
	Summary   string
}

// PrometheusRule converts the health alert into a Prometheus alerting rule
// with the labels that identify the resource and condition.
func (r Rule) PrometheusRule(kind string, resource metav1.Object) monitoringv1.Rule {
	duration := r.For
	return monitoringv1.Rule{
		Alert: r.Alert,
		Expr:  intstr.FromString(r.Expr),
		For:   &duration,
		Labels: map[string]string{
			LabelKind:      kind,
			LabelName:      resource.GetName(),
			LabelNamespace: resource.GetNamespace(),
			LabelCondition: string(r.Condition),
		},
		Annotations: map[string]string{
			AnnotationSummary: r.Summary,
		},
	}
}

// PrometheusRules converts health alerts of a resource into Prometheus alerting rules.
func PrometheusRules(rules []Rule, kind string, resource metav1.Object) []monitoringv1.Rule {
	converted := make([]monitoringv1.Rule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, rule.PrometheusRule(kind, resource))
	}
	return converted
}

// QuoteString quotes a value for use as a PromQL string literal.
func QuoteString(value string) string {
	return strconv.Quote(value)
}
