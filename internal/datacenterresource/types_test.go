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
	"reflect"
	"testing"

	chantico "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func typedResource(name, resourceType string) chantico.DataCenterResource {
	return chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       chantico.DataCenterResourceSpec{Type: resourceType},
	}
}

func TestDeriveLabels(t *testing.T) {
	site1 := typedResource("site1", DataCenterResourceTypeSite)
	site2 := typedResource("site2", DataCenterResourceTypeSite)
	pdu := typedResource("pdu1", DataCenterResourceTypePDU)
	bm := typedResource("bm1", DataCenterResourceTypeBaremetal)

	testCases := map[string]struct {
		resource  chantico.DataCenterResource
		ancestors []chantico.DataCenterResource
		expected  map[string]string
		err       error
	}{
		"no site": {
			resource:  bm,
			ancestors: []chantico.DataCenterResource{pdu},
		},
		"site ancestor": {
			resource:  bm,
			ancestors: []chantico.DataCenterResource{pdu, site1},
			expected:  map[string]string{siteLabel: "site1"},
		},
		"site itself": {
			resource: site1,
			expected: map[string]string{siteLabel: "site1"},
		},
		"multiple sites": {
			resource:  bm,
			ancestors: []chantico.DataCenterResource{site2, pdu, site1},
			err:       ErrorConflictingLabel{Label: siteLabel, Values: []string{"site1", "site2"}},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			labels, err := DeriveLabels(&tc.resource, tc.ancestors)
			if !reflect.DeepEqual(labels, tc.expected) || !reflect.DeepEqual(err, tc.err) {
				t.Errorf("DeriveLabels() = %#v, %v, want %#v, %v", labels, err, tc.expected, tc.err)
			}
		})
	}
}
