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
	"regexp"
	"testing"

	chantico "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRuleGroupPattern_MatchesOnlyOwnRuleGroup(t *testing.T) {
	resource := &chantico.DataCenterResource{ObjectMeta: metav1.ObjectMeta{Name: "rack.1", Namespace: "dc"}}
	pattern := regexp.MustCompile("^" + ruleGroupPattern(resource) + "$")
	file := "/etc/prometheus/rules/prometheus-prometheus-rulefiles-0/dc-rack.1-0b7c3d7e.yaml"

	tests := map[string]bool{
		file + ";chantico_rack_1":        true,
		file + ";chantico_rack_1_health": false,
		file + ";chantico_rack_10":       false,
		file + ";xchantico_rack_1":       false,
	}
	for ruleGroup, want := range tests {
		if got := pattern.MatchString(ruleGroup); got != want {
			t.Errorf("match %q = %v, want %v", ruleGroup, got, want)
		}
	}
}
