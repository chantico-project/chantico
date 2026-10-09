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

func testTemplateFrom(parameters ...chantico.TemplateParameter) *chantico.TemplateFrom {
	return &chantico.TemplateFrom{
		ValueSource: k8s.ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "coefficient-template"}, Key: "template",
		}},
		Parameters: parameters,
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := map[string]struct {
		text       string
		parameters []chantico.TemplateParameter
		objects    []runtime.Object
		want       string
	}{
		"inline parameter": {
			text:       `vm_cpu_percentage{vmid="{{ .vmid }}"}`,
			parameters: []chantico.TemplateParameter{{Name: "vmid", SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "3"}}},
			want:       `vm_cpu_percentage{vmid="3"}`,
		},
		"multiple parameters": {
			text: `vm_cpu_percentage{variable1="{{ .var1 }}", variable2="{{ .var2 }}"}`,
			parameters: []chantico.TemplateParameter{
				{Name: "var1", SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "3"}},
				{Name: "var2", SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "4"}},
			},
			want: `vm_cpu_percentage{variable1="3", variable2="4"}`,
		},
		"secret parameter": {
			text: `vm_cpu_percentage{vmid="{{ .vmid }}"}`,
			parameters: []chantico.TemplateParameter{{Name: "vmid", SecretConfigMapSelector: k8s.SecretConfigMapSelector{
				ValueFrom: &k8s.ValueSource{SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "vmid-secret"}, Key: "vmid",
				}},
			}}},
			objects: []runtime.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "vmid-secret", Namespace: "chantico"},
				Data:       map[string][]byte{"vmid": []byte("3")},
			}},
			want: `vm_cpu_percentage{vmid="3"}`,
		},
		"configmap parameter": {
			text: `vm_cpu_percentage{vmid="{{ .vmid }}"}`,
			parameters: []chantico.TemplateParameter{{Name: "vmid", SecretConfigMapSelector: k8s.SecretConfigMapSelector{
				ValueFrom: &k8s.ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "vmid-configmap"}, Key: "vmid",
				}},
			}}},
			objects: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "vmid-configmap", Namespace: "chantico"},
				Data:       map[string]string{"vmid": "3"},
			}},
			want: `vm_cpu_percentage{vmid="3"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			objects := append([]runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "coefficient-template", Namespace: "chantico"},
				Data:       map[string]string{"template": test.text},
			}}, test.objects...)
			renderer := testTemplateRenderer(t, objects...)
			got, err := renderer.renderTemplate(t.Context(), testTemplateFrom(test.parameters...))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("renderTemplate() = %q, want %q", got, test.want)
			}
		})
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
	parameter := chantico.TemplateParameter{Name: "identifier", SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "pdu1"}}
	tests := map[string]struct {
		resource *chantico.DataCenterResource
		want     ResolvedDataCenterResource
	}{
		"coefficient template": {
			resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{Name: "bm1", Namespace: "chantico"},
				Spec: chantico.DataCenterResourceSpec{
					Type:    DataCenterResourceTypeBaremetal,
					Parents: []chantico.ParentRef{{Name: "pdu1", CoefficientFrom: testTemplateFrom(parameter)}},
				},
			},
			want: ResolvedDataCenterResource{
				Name: "bm1", Type: DataCenterResourceTypeBaremetal,
				Parents: []ResolvedParent{{Name: "pdu1", Coefficient: `usage_coeff{identifier="pdu1"}`}},
			},
		},
		"energy metric template": {
			resource: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{Name: "pdu1", Namespace: "chantico"},
				Spec: chantico.DataCenterResourceSpec{
					Type: DataCenterResourceTypePDU,
					EnergyMetricFrom: &chantico.TemplateFrom{
						ValueSource: k8s.ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "energy-template"}, Key: "template",
						}},
						Parameters: []chantico.TemplateParameter{parameter},
					},
				},
			},
			want: ResolvedDataCenterResource{
				Name: "pdu1", Type: DataCenterResourceTypePDU,
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
