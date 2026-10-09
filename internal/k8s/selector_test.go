package k8s

import (
	"context"
	"fmt"
	"reflect"
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

func TestSecretConfigMapSelectorDeepCopy(t *testing.T) {
	t.Run("DeepCopy returns an independent deep copy", func(t *testing.T) {
		orig := &SecretConfigMapSelector{
			Value: "orig-value",
			ValueFrom: &ValueSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cm"},
					Key:                  "ck",
				},
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "s"},
					Key:                  "sk",
				},
			},
		}

		cp := orig.DeepCopy()
		if cp == nil {
			t.Fatalf("expected non-nil copy")
		}
		if !reflect.DeepEqual(orig, cp) {
			t.Fatalf("deep copy not equal to original:\norig=%#v\ncopy=%#v", orig, cp)
		}

		// mutate original
		orig.Value = "changed-value"
		orig.ValueFrom.ConfigMapKeyRef.Name = "cm2"
		orig.ValueFrom.ConfigMapKeyRef.Key = "ck2"
		orig.ValueFrom.SecretKeyRef.Name = "s2"
		orig.ValueFrom.SecretKeyRef.Key = "sk2"

		// copied should remain with original values
		if cp.ValueFrom == nil || cp.ValueFrom.ConfigMapKeyRef == nil || cp.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("unexpected nil in copy ValueFrom: %#v", cp)
		}
		if cp.ValueFrom.ConfigMapKeyRef.Name != "cm" || cp.ValueFrom.ConfigMapKeyRef.Key != "ck" {
			t.Fatalf("copy ConfigMapKeyRef mutated: %#v", cp.ValueFrom.ConfigMapKeyRef)
		}
		if cp.ValueFrom.SecretKeyRef.Name != "s" || cp.ValueFrom.SecretKeyRef.Key != "sk" {
			t.Fatalf("copy SecretKeyRef mutated: %#v", cp.ValueFrom.SecretKeyRef)
		}
	})

	t.Run("DeepCopyInto copies into provided target", func(t *testing.T) {
		in := &SecretConfigMapSelector{
			Value: "in-value",
			ValueFrom: &ValueSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cm-in"},
					Key:                  "k-in",
				},
			},
		}
		out := &SecretConfigMapSelector{}
		in.DeepCopyInto(out)
		if !reflect.DeepEqual(in, out) {
			t.Fatalf("DeepCopyInto result mismatch:\nin=%#v\nout=%#v", in, out)
		}

		// mutate source and ensure target unchanged
		in.Value = "changed"
		in.ValueFrom.ConfigMapKeyRef.Name = "cm-in-2"
		if out.Value != "in-value" || out.ValueFrom.ConfigMapKeyRef.Name != "cm-in" {
			t.Fatalf("out changed after in mutated: out=%#v", out)
		}
	})

	t.Run("DeepCopy handles nil receiver", func(t *testing.T) {
		var in *SecretConfigMapSelector
		if in.DeepCopy() != nil {
			t.Fatalf("expected nil when deepcopying nil receiver")
		}
	})
}

func TestValueSourceDeepCopy(t *testing.T) {
	t.Run("DeepCopy returns independent copy", func(t *testing.T) {
		vs := &ValueSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cm-vs"},
				Key:                  "k-vs",
			},
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "s-vs"},
				Key:                  "sk-vs",
			},
		}
		cp := vs.DeepCopy()
		if cp == nil {
			t.Fatalf("expected non-nil copy")
		}
		if !reflect.DeepEqual(vs, cp) {
			t.Fatalf("ValueSource deepcopy not equal:\norig=%#v\ncopy=%#v", vs, cp)
		}

		vs.ConfigMapKeyRef.Name = "cm-vs-2"
		vs.ConfigMapKeyRef.Key = "changed"
		vs.SecretKeyRef.Name = "s-vs-2"
		vs.SecretKeyRef.Key = "changed-sk"

		if cp.ConfigMapKeyRef.Name != "cm-vs" || cp.ConfigMapKeyRef.Key != "k-vs" {
			t.Fatalf("copy ConfigMapKeyRef mutated: %#v", cp.ConfigMapKeyRef)
		}
		if cp.SecretKeyRef.Name != "s-vs" || cp.SecretKeyRef.Key != "sk-vs" {
			t.Fatalf("copy SecretKeyRef mutated: %#v", cp.SecretKeyRef)
		}
	})

	t.Run("DeepCopy handles nil receiver", func(t *testing.T) {
		var vs *ValueSource
		if vs.DeepCopy() != nil {
			t.Fatalf("expected nil when deepcopying nil ValueSource")
		}
	})
}
