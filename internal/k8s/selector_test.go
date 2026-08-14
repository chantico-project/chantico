package k8s

import (
	"context"
	"fmt"
	"strings"
	"testing"

	errs "chantico/internal/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretConfigMapSelectorResolve(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "ns"},
		Data:       map[string]string{"the-key": "cm-value"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sec", Namespace: "ns"},
		Data:       map[string][]byte{"skey": []byte("secret-value")},
	}

	cases := []struct {
		Name        string
		Selector    SecretConfigMapSelector
		Objects     []client.Object
		Expected    string
		ExpectedErr error
	}{
		{
			Name:     "literal value",
			Selector: SecretConfigMapSelector{Value: "literal"},
			Expected: "literal",
		},
		{
			Name: "configmap",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"},
						Key:                  "the-key",
					},
				},
			},
			Objects:  []client.Object{cm},
			Expected: "cm-value",
		},
		{
			Name: "secret",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-sec"},
						Key:                  "skey",
					},
				},
			},
			Objects:  []client.Object{sec},
			Expected: "secret-value",
		},
		{
			Name: "configmap missing key",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"},
						Key:                  "missing",
					},
				},
			},
			Objects:     []client.Object{cm},
			ExpectedErr: &errs.MissingKeyError{Kind: errs.KindConfigMap, Name: "my-cm", Key: "missing"},
		},
		{
			Name: "secret missing key",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-sec"},
						Key:                  "missing",
					},
				},
			},
			Objects:     []client.Object{sec},
			ExpectedErr: &errs.MissingKeyError{Kind: errs.KindSecret, Name: "my-sec", Key: "missing"},
		},
		{
			Name: "empty valuefrom",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{},
			},
			ExpectedErr: &errs.MissingOptionError{Parent: "SecretConfigMapSelector", Options: []string{"Value", "ValueFrom.ConfigMapKeyRef", "ValueFrom.SecretKeyRef"}},
		},
		{
			Name: "configmap get error",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"},
						Key:                  "the-key",
					},
				},
			},
			// no objects -> get should return not found which should be wrapped
			ExpectedErr: &errs.GetError{Kind: errs.KindConfigMap, Name: "my-cm", Err: fmt.Errorf("")},
		},
		{
			Name: "secret get error",
			Selector: SecretConfigMapSelector{
				ValueFrom: &ValueSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-sec"},
						Key:                  "skey",
					},
				},
			},
			// no objects -> get should return not found which should be wrapped
			ExpectedErr: &errs.GetError{Kind: errs.KindSecret, Name: "my-sec", Err: fmt.Errorf("")},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tc.Objects) > 0 {
				builder = builder.WithObjects(tc.Objects...)
			}
			cl := builder.Build()

			got, err := tc.Selector.Resolve(ctx, cl, "ns")
			if tc.ExpectedErr != nil {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.ExpectedErr)
				}
				if !strings.Contains(err.Error(), tc.ExpectedErr.Error()) {
					t.Fatalf("expected error containing %q, got %v", tc.ExpectedErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.Expected {
				t.Fatalf("expected %q, got %q", tc.Expected, got)
			}
		})
	}
}
