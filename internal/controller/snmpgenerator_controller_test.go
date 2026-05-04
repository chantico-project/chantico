/*
Copyright 2025.

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

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/snmp"
	"chantico/internal/snmpgenerator"
	"chantico/internal/steps"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newReconciler(t *testing.T, root string, objs ...runtime.Object) *SnmpGeneratorReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := chantico.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
	return &SnmpGeneratorReconciler{
		Client: c,
		Scheme: scheme,
		Paths:  snmpgenerator.NewPaths(root),
	}
}

func TestReconcileGeneratorFile_WritesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	dev := &chantico.SNMPDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tno", Namespace: "chantico",
			UID: types.UID("dev-1"),
		},
		Spec: chantico.SNMPDeviceSpec{
			Auth:  snmp.GeneratorAuth{},
			Walks: []string{"1.3.6.1"},
		},
	}
	r := newReconciler(t, root, dev)

	// First run: writes the file.
	res := r.reconcileGeneratorFile(context.Background(), dev)
	if res.Action == steps.ActionError {
		t.Fatalf("first reconcileGeneratorFile errored: %v", res.Err)
	}
	path := r.Paths.GeneratorFile(dev.GetUID())
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}

	// Second run should not apply any changes.
	res = r.reconcileGeneratorFile(context.Background(), dev)
	if res.Action == steps.ActionError {
		t.Fatalf("second reconcileGeneratorFile errored: %v", res.Err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second run: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("file changed on idempotent run:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestReconcileMergedSNMPFile_WritesAtomically(t *testing.T) {
	root := t.TempDir()
	r := newReconciler(t, root)

	// Seed two per-device fragments.
	if err := os.MkdirAll(r.Paths.SNMPDir(), 0777); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(r.Paths.SNMPDir(), "snmp-a.yaml"),
		[]byte("auths: {foo: {version: 3}}\nmodules: {foo: {walk: [1.3]}}\n"))
	mustWrite(t, filepath.Join(r.Paths.SNMPDir(), "snmp-b.yaml"),
		[]byte("auths: {bar: {version: 3}}\nmodules: {bar: {walk: [1.4]}}\n"))

	dev := &chantico.SNMPDevice{ObjectMeta: metav1.ObjectMeta{Name: "tno", Namespace: "chantico"}}
	if res := r.reconcileMergedSNMPFile(context.Background(), dev); res.Action == steps.ActionError {
		t.Fatalf("reconcileMergedSNMPFile errored: %v", res.Err)
	}

	got, err := os.ReadFile(r.Paths.MergedSNMPFile())
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}

	wantHash := snmp.Hash(must(snmp.GetMergedSortedSNMPConfig(r.Paths.SNMPDir())))
	if snmp.Hash(got) != wantHash {
		t.Fatalf("merged file content does not match GetMergedSortedSNMPConfig output")
	}

	// No leftover .tmp file from the atomic rename.
	if _, err := os.Stat(r.Paths.MergedSNMPFile() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file leaked: %v", err)
	}
}

func mustWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
