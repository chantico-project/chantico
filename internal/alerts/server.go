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

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Server is a webhook server for Alertmanager notifications that keeps the manager running when it
// cannot start, for example when its certificates are missing.
type Server struct {
	webhook.Server
	Log logr.Logger
}

// NewServer creates a webhook server that only accepts clients with a certificate signed by the
// ca.crt in the certificate directory of the options.
func NewServer(options webhook.Options, log logr.Logger) *Server {
	options.ClientCAName = "ca.crt"
	return &Server{Server: webhook.NewServer(options), Log: log}
}

// Start runs the webhook server until the context is done, and logs instead of returning errors.
func (s *Server) Start(ctx context.Context) error {
	if err := s.Server.Start(ctx); err != nil {
		s.Log.Error(err, "Alert webhook server is not running; health conditions are not updated")
	}
	return nil
}
