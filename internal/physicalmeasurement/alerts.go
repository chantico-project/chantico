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

package physicalmeasurement

import (
	"fmt"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/alerts"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// HealthRuleName is the name of the PrometheusRule with the health alerts of a PhysicalMeasurement.
// It differs from the measurement name to avoid a conflict with the PrometheusRule of a DataCenterResource.
func HealthRuleName(measurement *chantico.PhysicalMeasurement) string {
	return "physicalmeasurement-" + measurement.Name
}

// ScrapePoolName is the scrape pool (job) name that the Prometheus Operator gives to the ScrapeConfig.
func ScrapePoolName(measurement *chantico.PhysicalMeasurement) string {
	return fmt.Sprintf("scrapeConfig/%s/%s", measurement.Namespace, measurement.Name)
}

// HealthRules returns the health alerts of a PhysicalMeasurement.
func HealthRules(measurement *chantico.PhysicalMeasurement) []alerts.Rule {
	return []alerts.Rule{
		{
			Alert:     "ChanticoScrapeConfigNotLoaded",
			Expr:      fmt.Sprintf("absent(prometheus_target_scrape_pool_targets{scrape_job=%s})", alerts.QuoteString(ScrapePoolName(measurement))),
			For:       "15s",
			Condition: chantico.ConditionScrapeConfigLoaded,
			Summary:   "Prometheus has not loaded the scrape configuration",
		},
		{
			Alert:     "ChanticoEndpointUnreachable",
			Expr:      fmt.Sprintf("up{name=%s, instance=%s} == 0", alerts.QuoteString(measurement.Name), alerts.QuoteString(measurement.Spec.Ip)),
			For:       "25s",
			Condition: chantico.ConditionEndpointReachable,
			Summary:   fmt.Sprintf("Scraping endpoint %s fails", measurement.Spec.Ip),
		},
	}
}

// HealthRuleSpec returns the PrometheusRule spec with the health alerts of a PhysicalMeasurement.
func HealthRuleSpec(measurement *chantico.PhysicalMeasurement) monitoringv1.PrometheusRuleSpec {
	return monitoringv1.PrometheusRuleSpec{
		Groups: []monitoringv1.RuleGroup{
			{
				Name:  "chantico_physicalmeasurement_health",
				Rules: alerts.PrometheusRules(HealthRules(measurement), alerts.KindPhysicalMeasurement, measurement),
			},
		},
	}
}
