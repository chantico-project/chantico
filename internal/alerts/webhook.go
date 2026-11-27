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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	chantico "chantico/api/v1alpha1"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WebhookPath is the path on the webhook server that receives Alertmanager notifications.
// Keep in sync with the AlertmanagerConfig receiver in chart/templates/alertmanager.yaml.
const WebhookPath = "/alertmanager/health"

const (
	statusFiring = "firing"
	maxBodyBytes = 1 << 20
)

// Message is the part of the Alertmanager webhook payload that is used.
// See https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type Message struct {
	Alerts []Alert `json:"alerts"`
}

// Alert is a single alert of an Alertmanager webhook payload.
type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type healthResource interface {
	client.Object
	HealthConditions() []chantico.ConditionType
	UpdateStatusCondition(chantico.ConditionType, metav1.ConditionStatus, chantico.ConditionReason, string)
}

type conditionTarget struct {
	kind      string
	key       types.NamespacedName
	condition chantico.ConditionType
}

type conditionUpdate struct {
	firing    bool
	summaries []string
}

// Handler updates the health conditions of Chantico resources from Alertmanager notifications.
// It never triggers retries of reconciliation; it only patches the conditions.
type Handler struct {
	Client client.Client
	Log    logr.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var message Message
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&message); err != nil {
		http.Error(w, "invalid alert payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	var errs []error
	for target, update := range collectUpdates(message.Alerts) {
		if err := h.apply(r.Context(), target, update); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		h.Log.Error(err, "Failed to update health conditions")
		// Alertmanager retries the notification on a server error.
		http.Error(w, "failed to update health conditions", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// collectUpdates combines the alerts per resource condition: the condition is unhealthy when any of its alerts fire.
func collectUpdates(alerts []Alert) map[conditionTarget]*conditionUpdate {
	updates := make(map[conditionTarget]*conditionUpdate)
	for _, alert := range alerts {
		target := conditionTarget{
			kind:      alert.Labels[LabelKind],
			key:       types.NamespacedName{Namespace: alert.Labels[LabelNamespace], Name: alert.Labels[LabelName]},
			condition: chantico.ConditionType(alert.Labels[LabelCondition]),
		}
		if target.kind == "" || target.key.Name == "" || target.key.Namespace == "" || target.condition == "" {
			continue
		}
		update, ok := updates[target]
		if !ok {
			update = &conditionUpdate{}
			updates[target] = update
		}
		if alert.Status == statusFiring {
			update.firing = true
			summary := alert.Annotations[AnnotationSummary]
			if summary == "" {
				summary = alert.Labels["alertname"]
			}
			if !slices.Contains(update.summaries, summary) {
				update.summaries = append(update.summaries, summary)
			}
		}
	}
	return updates
}

func newResource(kind string) healthResource {
	switch kind {
	case KindPhysicalMeasurement:
		return &chantico.PhysicalMeasurement{}
	case KindDataCenterResource:
		return &chantico.DataCenterResource{}
	default:
		return nil
	}
}

func (h *Handler) apply(ctx context.Context, target conditionTarget, update *conditionUpdate) error {
	log := h.Log.WithValues("kind", target.kind, "resource", target.key, "condition", target.condition)
	resource := newResource(target.kind)
	if resource == nil {
		log.Info("Ignoring alert for unknown kind")
		return nil
	}
	if err := h.Client.Get(ctx, target.key, resource); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Ignoring alert for resource that does not exist")
			return nil
		}
		return fmt.Errorf("get %s %s: %w", target.kind, target.key, err)
	}
	if !slices.Contains(resource.HealthConditions(), target.condition) {
		log.Info("Ignoring alert for condition that is not a health condition")
		return nil
	}

	patcher, err := patch.NewHelper(resource, h.Client)
	if err != nil {
		return err
	}
	if update.firing {
		slices.Sort(update.summaries)
		resource.UpdateStatusCondition(target.condition, metav1.ConditionFalse, chantico.ReasonAlertFiring, strings.Join(update.summaries, "; "))
	} else {
		resource.UpdateStatusCondition(target.condition, metav1.ConditionTrue, chantico.ReasonAlertResolved, "Health alert resolved")
	}
	log.Info("Updating health condition", "firing", update.firing)
	if err := patcher.Patch(ctx, resource, patch.WithOwnedConditions{Conditions: []string{string(target.condition)}}); err != nil {
		return fmt.Errorf("patch %s %s: %w", target.kind, target.key, err)
	}
	return nil
}
