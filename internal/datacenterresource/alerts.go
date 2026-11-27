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

package datacenterresource

import (
	"fmt"
	"regexp"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/alerts"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// RecordingRuleGroupName is the name of the rule group with the recording rules of a resource.
func RecordingRuleGroupName(resource *chantico.DataCenterResource) string {
	return fmt.Sprintf("chantico_%s", SanitizeMetricName(resource.Name))
}

// ruleGroupPattern matches the rule_group label of Prometheus rule metrics, "<rule file>;<group name>",
// for the recording rules of the resource.
func ruleGroupPattern(resource *chantico.DataCenterResource) string {
	return ".*;" + regexp.QuoteMeta(RecordingRuleGroupName(resource))
}

// HealthRules returns the health alerts of a DataCenterResource.
func HealthRules(resource *chantico.DataCenterResource) []alerts.Rule {
	return []alerts.Rule{
		{
			Alert:     "ChanticoRecordingRuleNoData",
			Expr:      fmt.Sprintf("absent(%s{resource=%s})", EnergyMetricName, alerts.QuoteString(resource.Name)),
			For:       "2m",
			Condition: chantico.ConditionRecordingRuleHealthy,
			Summary:   fmt.Sprintf("Recording rules produce no %s data", EnergyMetricName),
		},
		{
			Alert: "ChanticoRecordingRuleFailing",
			Expr: fmt.Sprintf("increase(prometheus_rule_evaluation_failures_total{rule_group=~%s}[5m]) > 0",
				alerts.QuoteString(ruleGroupPattern(resource))),
			For:       "0s",
			Condition: chantico.ConditionRecordingRuleHealthy,
			Summary:   "Recording rule evaluation fails",
		},
	}
}

// HealthRuleGroup returns the rule group with the health alerts of a DataCenterResource.
func HealthRuleGroup(resource *chantico.DataCenterResource) monitoringv1.RuleGroup {
	return monitoringv1.RuleGroup{
		Name:  RecordingRuleGroupName(resource) + "_health",
		Rules: alerts.PrometheusRules(HealthRules(resource), alerts.KindDataCenterResource, resource),
	}
}
