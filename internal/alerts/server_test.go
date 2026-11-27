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
	"testing"

	"github.com/go-logr/logr/funcr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func TestServerStart_MissingCertificatesLogsWithoutError(t *testing.T) {
	logged := ""
	log := funcr.New(func(prefix, args string) { logged += args }, funcr.Options{})
	server := NewServer(webhook.Options{CertDir: t.TempDir(), Port: 0}, log)

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if logged == "" {
		t.Error("Start() did not log the error")
	}
}
