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

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"chantico/api/v1alpha1"
	"chantico/internal/testutil"
	"chantico/internal/webapp/internal/graph"
	"chantico/internal/webapp/internal/html"
	"chantico/internal/webapp/internal/kubernetes"
)

func newTestServer(t *testing.T, objs ...client.Object) http.Handler {
	t.Helper()
	c := testutil.NewFakeClientBuilder(t, corev1.AddToScheme, v1alpha1.AddToScheme).
		WithObjects(objs...).
		Build()
	renderer, err := html.New()
	if err != nil {
		t.Fatalf("html.New() unexpected error: %v", err)
	}
	h := &HTTPServerHandler{
		Renderer:   renderer,
		Kubernetes: kubernetes.NewWithClient(c, "kind-test", "https://cluster.example"),
	}
	return New(h, &HTTPServerConfig{Port: 0}).server.Handler
}

func newDCR(namespace, name string, parents ...string) *v1alpha1.DataCenterResource {
	d := &v1alpha1.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       v1alpha1.DataCenterResourceSpec{Type: "pdu", EnergyMetric: "metric"},
	}
	for _, p := range parents {
		d.Spec.Parents = append(d.Spec.Parents, v1alpha1.ParentRef{Name: p, Coefficient: "1"})
	}
	return d
}

func serve(t *testing.T, h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGraphAPI(t *testing.T) {
	h := newTestServer(t,
		newDCR("ns1", "root"),
		newDCR("ns1", "child", "root"),
		newDCR("ns2", "other"),
	)

	rec := serve(t, h, http.MethodGet, "/api/graph?namespace=ns1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var elements graph.Elements
	if err := json.Unmarshal(rec.Body.Bytes(), &elements); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(elements.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(elements.Nodes))
	}
	if len(elements.Edges) != 1 || elements.Edges[0].Data["source"] != "ns1-child" || elements.Edges[0].Data["target"] != "ns1-root" {
		t.Errorf("edges = %+v, want one edge ns1-child -> ns1-root", elements.Edges)
	}
}

func TestNamespacesAPI(t *testing.T) {
	h := newTestServer(t,
		newDCR("beta", "a"),
		newDCR("alpha", "a"),
	)

	rec := serve(t, h, http.MethodGet, "/api/namespaces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var namespaces []string
	if err := json.Unmarshal(rec.Body.Bytes(), &namespaces); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(namespaces) != 2 || namespaces[0] != "alpha" || namespaces[1] != "beta" {
		t.Errorf("namespaces = %v, want [alpha beta]", namespaces)
	}
}

func TestNodeAPI(t *testing.T) {
	h := newTestServer(t, newDCR("ns1", "pdu-a"))

	t.Run("found", func(t *testing.T) {
		rec := serve(t, h, http.MethodGet, "/api/nodes/ns1-pdu-a", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{"<h2>pdu-a</h2>", `hx-put="/api/nodes/ns1-pdu-a"`, "energyMetric: metric"} {
			if !strings.Contains(body, want) {
				t.Errorf("body missing %q", want)
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := serve(t, h, http.MethodGet, "/api/nodes/ns9-missing", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestUpdateNodeAPI(t *testing.T) {
	manifest := "metadata:\n  name: pdu-a\n  namespace: ns1\nspec:\n  type: pdu\n  energyMetric: updated\n"

	tests := []struct {
		name       string
		id         string
		manifest   string
		wantStatus int
	}{
		{"success", "ns1-pdu-a", manifest, http.StatusOK},
		{"not found", "ns9-missing", manifest, http.StatusNotFound},
		{"invalid manifest", "ns1-pdu-a", "spec: [unclosed", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestServer(t, newDCR("ns1", "pdu-a"))

			rec := serve(t, h, http.MethodPut, "/api/nodes/"+tt.id, url.Values{"manifest": {tt.manifest}})
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(rec.Body.String(), "updated") {
				t.Errorf("body does not reflect updated metric: %s", rec.Body.String())
			}
		})
	}
}

func TestDeleteNodeAPI(t *testing.T) {
	h := newTestServer(t, newDCR("ns1", "pdu-a"))

	rec := serve(t, h, http.MethodDelete, "/api/nodes/ns1-pdu-a", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "node-deleted" {
		t.Errorf("HX-Trigger = %q, want node-deleted", got)
	}

	rec = serve(t, h, http.MethodDelete, "/api/nodes/ns1-pdu-a", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", rec.Code)
	}
}

func TestHomePage(t *testing.T) {
	h := newTestServer(t, newDCR("ns1", "pdu-a"))

	rec := serve(t, h, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"https://cluster.example", "kind-test", `<option value="ns1">ns1</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q", want)
		}
	}
}

func TestStaticFiles(t *testing.T) {
	h := newTestServer(t)

	rec := serve(t, h, http.MethodGet, "/static/style.css", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
