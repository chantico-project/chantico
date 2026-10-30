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

// Package testutil provides shared helpers for tests.
package testutil

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewFakeClientBuilder creates an isolated fake-client builder with the supplied
// scheme registrations. Tests can add objects or interceptors before calling Build.
func NewFakeClientBuilder(t testing.TB, addToScheme ...func(*runtime.Scheme) error) *fake.ClientBuilder {
	t.Helper()

	scheme := runtime.NewScheme()
	for _, register := range addToScheme {
		if err := register(scheme); err != nil {
			t.Fatalf("register fake client scheme: %v", err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme)
}
