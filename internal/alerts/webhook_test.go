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

package alerts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/testutil"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func setupWebhookTest(t *testing.T) (*Handler, *chantico.DataCenterResource) {
	t.Helper()
	resource := &chantico.DataCenterResource{ObjectMeta: metav1.ObjectMeta{Name: "rack", Namespace: "chantico"}}
	resource.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "")
	resource.UpdateStatusCondition(chantico.ConditionRecordingRuleHealthy, metav1.ConditionTrue, chantico.ReasonReconciled, "")
	kubeClient := testutil.NewFakeClientBuilder(t, chantico.AddToScheme).
		WithStatusSubresource(&chantico.DataCenterResource{}).
		WithObjects(resource).
		Build()
	return &Handler{Client: kubeClient, Log: logr.Discard()}, resource
}

func alertJSON(status, condition, summary string) string {
	return `{"status":"` + status + `","labels":{"alertname":"Test","chantico_kind":"DataCenterResource",` +
		`"chantico_name":"rack","namespace":"chantico","chantico_condition":"` + condition + `"},` +
		`"annotations":{"summary":"` + summary + `"}}`
}

func postAlerts(t *testing.T, handler *Handler, alerts ...string) int {
	t.Helper()
	body := `{"version":"4","status":"firing","alerts":[` + strings.Join(alerts, ",") + `]}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body)))
	return recorder.Code
}

func healthCondition(t *testing.T, handler *Handler, resource *chantico.DataCenterResource) *metav1.Condition {
	t.Helper()
	stored := &chantico.DataCenterResource{}
	if err := handler.Client.Get(t.Context(), client.ObjectKeyFromObject(resource), stored); err != nil {
		t.Fatal(err)
	}
	if !meta.IsStatusConditionTrue(stored.Status.Conditions, string(chantico.ConditionReady)) {
		t.Errorf("webhook changed a controller condition: %v", stored.Status.Conditions)
	}
	return meta.FindStatusCondition(stored.Status.Conditions, string(chantico.ConditionRecordingRuleHealthy))
}

func TestWebhook_FiringAndResolved(t *testing.T) {
	handler, resource := setupWebhookTest(t)
	condition := string(chantico.ConditionRecordingRuleHealthy)

	if code := postAlerts(t, handler, alertJSON("firing", condition, "no data"), alertJSON("resolved", condition, "failing")); code != http.StatusOK {
		t.Fatalf("status code = %d", code)
	}
	got := healthCondition(t, handler, resource)
	if got.Status != metav1.ConditionFalse || got.Reason != string(chantico.ReasonAlertFiring) || got.Message != "no data" {
		t.Fatalf("unexpected condition after firing alert: %+v", got)
	}

	if code := postAlerts(t, handler, alertJSON("resolved", condition, "no data")); code != http.StatusOK {
		t.Fatalf("status code = %d", code)
	}
	got = healthCondition(t, handler, resource)
	if got.Status != metav1.ConditionTrue || got.Reason != string(chantico.ReasonAlertResolved) {
		t.Fatalf("unexpected condition after resolved alert: %+v", got)
	}
}

func TestWebhook_IgnoresUnknownTargets(t *testing.T) {
	handler, resource := setupWebhookTest(t)
	missing := strings.Replace(alertJSON("firing", string(chantico.ConditionRecordingRuleHealthy), "x"), `"rack"`, `"missing"`, 1)
	notHealth := alertJSON("firing", string(chantico.ConditionReady), "x")
	unlabelled := `{"status":"firing","labels":{"alertname":"Other"}}`

	if code := postAlerts(t, handler, missing, notHealth, unlabelled); code != http.StatusOK {
		t.Fatalf("status code = %d", code)
	}
	if got := healthCondition(t, handler, resource); got.Status != metav1.ConditionTrue {
		t.Fatalf("unexpected condition: %+v", got)
	}
}

func TestWebhook_RejectsInvalidRequests(t *testing.T) {
	handler, _ := setupWebhookTest(t)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, WebhookPath, nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status code = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader("{")))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("invalid payload status code = %d", recorder.Code)
	}
}
