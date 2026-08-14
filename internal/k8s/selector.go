package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	errs "chantico/internal/errors"
)

type SecretConfigMapSelector corev1.EnvVar
type ValueSource = corev1.EnvVarSource

func (s SecretConfigMapSelector) Resolve(ctx context.Context, r client.Client, namespace string) (string, error) {
	// return the literal value if ValueFrom is not provided
	if s.ValueFrom == nil {
		return s.Value, nil
	}
	if ref := s.ValueFrom.ConfigMapKeyRef; ref != nil {
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
	if ref := s.ValueFrom.SecretKeyRef; ref != nil {
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
	return "", &errs.MissingOptionError{Parent: "SecretConfigMapSelector", Options: []string{"Value", "ValueFrom.ConfigMapKeyRef", "ValueFrom.SecretKeyRef"}}
}
