package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConditionType string

const (
	ConditionReady     ConditionType = "Ready"     // Used in both DataCenterResource and MeasurementDevice CRDs. True only when fully converged.
	ConditionValidated ConditionType = "Validated" // Used in DataCenterResource and PhysicalMeasurement CRDs. True only when the spec has been validated.
	ConditionGenerated ConditionType = "Generated" // Used in MeasurementDevice CRD. True only when the spec has been generated.
	ConditionApplied   ConditionType = "Applied"   // Used in Both DataCenterResource and MeasurementDevice CRDs. True only when runtime config has been written/applied.
)

type ConditionReason string

const (
	ReasonReconciling              ConditionReason = "Reconciling"
	ReasonReconciled               ConditionReason = "Reconciled"
	ReasonInvalidSpec              ConditionReason = "InvalidSpec"
	ReasonDependencyUnavailable    ConditionReason = "DependencyUnavailable"
	ReasonGenerationPending        ConditionReason = "GenerationPending"
	ReasonGenerationFailed         ConditionReason = "GenerationFailed"
	ReasonApplyFailed              ConditionReason = "ApplyFailed"
	ReasonCleanupFailed            ConditionReason = "CleanupFailed"
	ReasonReloadFailed             ConditionReason = "ReloadFailed"
	ReasonTemplateResolutionFailed ConditionReason = "TemplateResolutionFailed"
)

// +kubebuilder:object:generate=false
type ConditionsObject interface {
	metav1.Object
	GetConditions() *[]metav1.Condition
	UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string)
}

func updateStatusCondition(o ConditionsObject, t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(o.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: o.GetGeneration(),
	})
}

type ConditionedStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
