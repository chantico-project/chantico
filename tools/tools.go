//go:build tools

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

// Package tools makes go.mod require the tools directly instead of as
// indirect, so that Dependabot updates them.
package tools

import (
	_ "github.com/go-semantic-release/semantic-release/v2/cmd/semantic-release"
	_ "github.com/gohugoio/hugo"
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	_ "github.com/t-yuki/gocover-cobertura"
	_ "github.com/wjdp/htmltest"
	_ "go.abhg.dev/doc2go"
	_ "gotest.tools/gotestsum"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kind"
	_ "sigs.k8s.io/kustomize/kustomize/v5"
)
