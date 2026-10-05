package webapp

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

import "testing"

func TestLoadConfigKubeconfigPath(t *testing.T) {
	kubeconfigPath := t.TempDir()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "local default", want: expandPath("~/.kube/config")},
		{name: "in cluster", env: map[string]string{"KUBERNETES_SERVICE_HOST": "kubernetes.default.svc"}},
		{name: "explicit kubeconfig in cluster", env: map[string]string{
			"KUBERNETES_SERVICE_HOST": "kubernetes.default.svc",
			"KUBECONFIG":              kubeconfigPath,
		}, want: kubeconfigPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := loadConfig(func(key string) string { return test.env[key] })
			if err != nil {
				t.Fatal(err)
			}
			if cfg.KubeconfigPath != test.want {
				t.Fatalf("KubeconfigPath = %q, want %q", cfg.KubeconfigPath, test.want)
			}
		})
	}
}
