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

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testTemplateClient(t *testing.T, objects ...runtime.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
}

func testTemplateFrom(parameters ...TemplateParameter) *TemplateFrom {
	return &TemplateFrom{
		SecretConfigMapSelector: SecretConfigMapSelector{ValueFrom: &ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "coefficient-template"}, Key: "template",
		}}},
		Parameters: parameters,
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := map[string]struct {
		text       string
		parameters []TemplateParameter
		objects    []runtime.Object
		want       string
	}{
		"inline parameter": {
			text:       `vm_cpu_percentage{vmid="{{ .vmid }}"}`,
			parameters: []TemplateParameter{{Name: "vmid", SecretConfigMapSelector: SecretConfigMapSelector{Value: "3"}}},
			want:       `vm_cpu_percentage{vmid="3"}`,
		},
		"multiple parameters": {
			text: `vm_cpu_percentage{variable1="{{ .var1 }}", variable2="{{ .var2 }}"}`,
			parameters: []TemplateParameter{
				{Name: "var1", SecretConfigMapSelector: SecretConfigMapSelector{Value: "3"}},
				{Name: "var2", SecretConfigMapSelector: SecretConfigMapSelector{Value: "4"}},
			},
			want: `vm_cpu_percentage{variable1="3", variable2="4"}`,
		},
		"secret parameter": {
			text: `vm_cpu_percentage{vmid="{{ .vmid }}"}`,
			parameters: []TemplateParameter{{Name: "vmid", SecretConfigMapSelector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{SecretKeyRef: &corev1.SecretKeySelector{
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
			parameters: []TemplateParameter{{Name: "vmid", SecretConfigMapSelector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
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
			got, err := testTemplateFrom(test.parameters...).Render(t.Context(), testTemplateClient(t, objects...), "chantico")
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("Render() = %q, want %q", got, test.want)
			}
		})
	}
}
