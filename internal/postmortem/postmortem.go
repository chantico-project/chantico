package log

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	chantico "chantico/api/v1alpha1"
	vol "chantico/internal/volumes"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = k8sruntime.NewScheme()
)

type Markdown interface {
	Markdown()
}

type ClusterState struct {
	CRDStates []any
}

func (cs *ClusterState) Markdown() string {
	template := `
- CRDs state:
` + "```" + `
%#v
` + "```" + `
`
	return fmt.Sprintf(template, cs.CRDStates)
}

type ChanticoState struct {
	Error           error
	File            string
	Line            int
	FunctionName    string
	Stack           string
	LoggedVariables []any
}

func (cs *ChanticoState) Markdown() string {
	template := `
- File: %s
- Line: %d
- FunctionName: %s
- Error:
` + "```" + `
%s
` + "```" + `
- Stack:
` + "```" + `
%s
` + "```" + `
- Logged variables:
` + "```" + `
%#v
` + "```" + `
`
	return fmt.Sprintf(template, cs.File, cs.Line, cs.FunctionName, cs.Error, cs.Stack, cs.LoggedVariables)
}

type PostMortem struct {
	Timestamp     time.Time
	ClusterState  ClusterState
	ChanticoState ChanticoState
}

func NewPostMortem(err error, args ...any) *PostMortem {
	// Get Chantico current state
	var ok bool
	var pc uintptr
	chanticoState := ChanticoState{Error: err, Stack: string(debug.Stack()), LoggedVariables: args}

	pc, chanticoState.File, chanticoState.Line, ok = runtime.Caller(1)
	if !ok {
		return nil
	}

	fn := runtime.FuncForPC(pc)
	if fn != nil {
		chanticoState.FunctionName = fn.Name()
	}

	// Get the cluster state
	clusterState := ClusterState{}
	chantico.AddToScheme(scheme)

	cfg := ctrl.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil
	}

	measurementDevices := &chantico.MeasurementDeviceList{}
	err = c.List(context.TODO(), measurementDevices, client.InNamespace("chantico"))
	if err != nil {
		clusterState.CRDStates = append(clusterState.CRDStates, "Could not find measurementDevices in the chantico namespace")
	}
	clusterState.CRDStates = append(clusterState.CRDStates, measurementDevices.Items)

	return &PostMortem{
		ChanticoState: chanticoState,
		ClusterState:  clusterState,
	}
}

func (pm *PostMortem) Markdown() string {
	// This template is based on the issue template developped by Jeroen
	template := `
---
name: 🐛 Bug Report
about: Report a problem or unexpected behavior in the system
title: '[BUG] '
labels: bug
assignees: ''

---

## 🐞 Description

A clear and concise description of what the bug is, where it happens, and what the expected behavior should be.

---

## 🔁 How to Reproduce

### Cluster state

%s

### Chantico state

%s

---

## 🧪 Suggested Testing or Validation

Explain how the fix can be verified. Mention test cases, test environments, or steps to revalidate.

---

## 📂 Logs, Screenshots, or Code Snippets

Include logs, screenshots, or relevant code snippets to support the bug report.

---
`
	return fmt.Sprintf(template, pm.ClusterState.Markdown(), pm.ChanticoState.Markdown())
}

func (pm *PostMortem) SaveAndQuit() {
	dir := fmt.Sprintf("%s/bugs", os.Getenv(vol.ChanticoVolumeLocationEnv))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(fmt.Sprintf("Could not create post-mortem folder %s", dir))
	}

	filename := fmt.Sprintf("%s/bug%d.md", dir, pm.Timestamp.UnixMicro())
	err = os.WriteFile(filename, []byte(pm.Markdown()), 0666)
	if err != nil {
		panic(fmt.Sprintf("Could not save postmortem at location %s\nPost mortem content:%s\n", filename, pm.Markdown()))
	}

	panic(fmt.Sprintf("New postmortem generated and saved at %s", filename))
}
