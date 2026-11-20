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
	"chantico/internal/k8s"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testTemplateRenderer(t *testing.T, objects ...runtime.Object) *TemplateRenderer {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return NewTemplateRenderer(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(), "chantico")
}

func testTemplateFrom(parameters ...k8s.TemplateParameter) *k8s.TemplateFrom {
	return &k8s.TemplateFrom{
		SecretConfigMapSelector: k8s.SecretConfigMapSelector{ValueFrom: &k8s.ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "coefficient-template"}, Key: "template",
		}}},
		TemplateParameters: parameters,
	}
}

func TestResolvedExpressions(t *testing.T) {
	renderer := testTemplateRenderer(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coefficient-template", Namespace: "chantico"},
		Data:       map[string]string{"template": `usage_coeff{identifier="{{ .identifier }}"}`},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "energy-template", Namespace: "chantico"},
		Data:       map[string]string{"template": `power_watts{identifier="{{ .identifier }}"}`},
	})
	parameter := k8s.TemplateParameter{Name: "identifier", SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "pdu1"}}
	tests := map[string]struct {
		resource *chantico.DataCenterResource
		want     ResolvedExpressions
	}{
		"coefficient template": {
			resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{Name: "bm1", Namespace: "chantico"},
				Spec: chantico.DataCenterResourceSpec{
					Type:    DataCenterResourceTypeBaremetal,
					Parents: []chantico.ParentRef{{Name: "pdu1", CoefficientFrom: testTemplateFrom(parameter)}},
				},
			},
			want: ResolvedExpressions{
				Parents: []ResolvedParent{{Name: "pdu1", Coefficient: `usage_coeff{identifier="pdu1"}`}},
			},
		},
		"energy metric template": {
			resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{Name: "pdu1", Namespace: "chantico"},
				Spec: chantico.DataCenterResourceSpec{
					Type: DataCenterResourceTypePDU,
					EnergyMetricFrom: &k8s.TemplateFrom{
						SecretConfigMapSelector: k8s.SecretConfigMapSelector{ValueFrom: &k8s.ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "energy-template"}, Key: "template",
						}}},
						TemplateParameters: []k8s.TemplateParameter{parameter},
					},
				},
			},
			want: ResolvedExpressions{
				EnergyMetric: `power_watts{identifier="pdu1"}`,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := renderer.ResolvedExpressions(t.Context(), test.resource)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, test.want) {
				t.Errorf("ResolvedExpressions() = %+v, want %+v", *got, test.want)
			}
		})
	}
}
