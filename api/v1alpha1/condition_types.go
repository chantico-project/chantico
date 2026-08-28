package v1alpha1

type ConditionType string

const (
	ConditionReady     ConditionType = "Ready"     // Used in both DataCenterResource and MeasurementDevice CRDs. True only when fully converged.
	ConditionValidated ConditionType = "Validated" // Used in DataCenterResource CRD. True only when the spec has been validated.
	ConditionGenerated ConditionType = "Generated" // Used in MeasurementDevice CRD. True only when the spec has been generated.
	ConditionApplied   ConditionType = "Applied"   // Used in Both DataCenterResource and MeasurementDevice CRDs. True only when runtime config has been written/applied.
)

type ConditionReason string

const (
	ReasonReconciling           ConditionReason = "Reconciling"
	ReasonReconciled            ConditionReason = "Reconciled"
	ReasonInvalidSpec           ConditionReason = "InvalidSpec"
	ReasonDependencyUnavailable ConditionReason = "DependencyUnavailable"
	ReasonGenerationPending     ConditionReason = "GenerationPending"
	ReasonGenerationFailed      ConditionReason = "GenerationFailed"
	ReasonApplyFailed           ConditionReason = "ApplyFailed"
	ReasonCleanupFailed         ConditionReason = "CleanupFailed"
	ReasonReloadFailed          ConditionReason = "ReloadFailed"
)
