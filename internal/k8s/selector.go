package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	errs "chantico/internal/errors"
)

// +kubebuilder:validation:XValidation:rule="!(has(self.value) && has(self.valueFrom))",message="Value and ValueFrom cannot be set at the same time"
// SecretConfigMapSelector is a generic selector that resolves to a string value
// coming from one of: a literal Value, a key in a ConfigMap, or a key in a Secret.
type SecretConfigMapSelector struct {
	// Value is a literal value for the selector.
	// Mutually exclusive with ValueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// ValueFrom selects a source for the selector's value. Supported sources are
	// limited to ConfigMapKeyRef and SecretKeyRef.
	// +optional
	ValueFrom *ValueSource `json:"valueFrom,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!(has(self.configMapKeyRef) && has(self.secretKeyRef))",message="ConfigMapKeyRef and SecretKeyRef cannot be set simultaneously"
// ValueSource is a limited representation containing only the fields used by
// this controller (ConfigMapKeyRef and SecretKeyRef).
type ValueSource struct {
	// Selects a key of a ConfigMap to populate the value.
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a Secret to populate the value.
	// +optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

func fetchFromConfigMap(ctx context.Context, r client.Client, namespace string, ref *corev1.ConfigMapKeySelector) (string, error) {
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, configMap); err != nil {
		return "", &errs.GetError{Kind: errs.KindConfigMap, Name: ref.Name, Err: err}
	}
	value, ok := configMap.Data[ref.Key]
	if !ok {
		return "", &errs.MissingKeyError{Kind: errs.KindConfigMap, Name: ref.Name, Key: ref.Key}

	}
	return value, nil
}

func fetchFromSecret(ctx context.Context, r client.Client, namespace string, ref *corev1.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return "", &errs.GetError{Kind: errs.KindSecret, Name: ref.Name, Err: err}

	}
	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", &errs.MissingKeyError{Kind: errs.KindSecret, Name: ref.Name, Key: ref.Key}
	}
	return string(value), nil
}

func (s ValueSource) Resolve(ctx context.Context, r client.Client, namespace string) (string, error) {
	if ref := s.ConfigMapKeyRef; ref != nil {
		return fetchFromConfigMap(ctx, r, namespace, ref)
	}
	if ref := s.SecretKeyRef; ref != nil {
		return fetchFromSecret(ctx, r, namespace, ref)
	}
	return "", &errs.MissingOptionError{Parent: "ValueSource", Options: []string{"ConfigMapKeyRef", "SecretKeyRef"}}
}

func (s SecretConfigMapSelector) Resolve(ctx context.Context, r client.Client, namespace string) (string, error) {
	// return the literal value if ValueFrom is not provided
	if s.ValueFrom == nil {
		return s.Value, nil
	}
	if s.ValueFrom.ConfigMapKeyRef == nil && s.ValueFrom.SecretKeyRef == nil {
		return "", &errs.MissingOptionError{Parent: "SecretConfigMapSelector", Options: []string{"Value", "ValueFrom.ConfigMapKeyRef", "ValueFrom.SecretKeyRef"}}
	}
	return s.ValueFrom.Resolve(ctx, r, namespace)
}

// DeepCopyInto implements a manual deep-copy for SecretConfigMapSelector
func (in *SecretConfigMapSelector) DeepCopyInto(out *SecretConfigMapSelector) {
	*out = *in
	if in.ValueFrom != nil {
		in, out := &in.ValueFrom, &out.ValueFrom
		*out = new(ValueSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy creates a new deep-copied SecretConfigMapSelector.
func (in *SecretConfigMapSelector) DeepCopy() *SecretConfigMapSelector {
	if in == nil {
		return nil
	}
	out := new(SecretConfigMapSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto implements a manual deep-copy for ValueSource.
func (in *ValueSource) DeepCopyInto(out *ValueSource) {
	*out = *in
	if in.ConfigMapKeyRef != nil {
		in, out := &in.ConfigMapKeyRef, &out.ConfigMapKeyRef
		*out = new(corev1.ConfigMapKeySelector)
		**out = **in
	}
	if in.SecretKeyRef != nil {
		in, out := &in.SecretKeyRef, &out.SecretKeyRef
		*out = new(corev1.SecretKeySelector)
		**out = **in
	}
}

// DeepCopy creates a new deep-copied ValueSource.
func (in *ValueSource) DeepCopy() *ValueSource {
	if in == nil {
		return nil
	}
	out := new(ValueSource)
	in.DeepCopyInto(out)
	return out
}
