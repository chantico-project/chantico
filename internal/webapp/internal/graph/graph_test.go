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

package graph

import "testing"

func TestBuildElements(t *testing.T) {
	parentA := &Node{Name: "ns-a", Label: "a", Status: "Ready"}
	parentB := &Node{Name: "ns-b", Label: "b", Status: "Ready"}

	tests := []struct {
		name      string
		nodes     []*Node
		wantNodes []map[string]string
		wantEdges []map[string]string
	}{
		{
			name:      "nil input",
			nodes:     nil,
			wantNodes: []map[string]string{},
			wantEdges: []map[string]string{},
		},
		{
			name:  "single node without parents",
			nodes: []*Node{{Name: "ns-child", Label: "child", Status: "Ready"}},
			wantNodes: []map[string]string{
				{"id": "ns-child", "label": "child", "type": "", "status": "Ready"},
			},
			wantEdges: []map[string]string{},
		},
		{
			name: "node with two parents",
			nodes: []*Node{{
				Name:    "ns-child",
				Label:   "child",
				Status:  "Pending",
				Parents: []*Node{parentA, parentB},
			}},
			wantNodes: []map[string]string{
				{"id": "ns-child", "label": "child", "type": "", "status": "Pending"},
			},
			wantEdges: []map[string]string{
				{"id": "ns-child->ns-a", "source": "ns-child", "target": "ns-a"},
				{"id": "ns-child->ns-b", "source": "ns-child", "target": "ns-b"},
			},
		},
		{
			name: "multiple nodes with unregistered parent",
			nodes: []*Node{
				{Name: "ns-a", Label: "a", Status: "Ready"},
				{
					Name:    "ns-c",
					Label:   "c",
					Status:  "Faulty",
					Parents: []*Node{{Name: "ns-missing"}},
				},
			},
			wantNodes: []map[string]string{
				{"id": "ns-a", "label": "a", "type": "", "status": "Ready"},
				{"id": "ns-c", "label": "c", "type": "", "status": "Faulty"},
			},
			wantEdges: []map[string]string{
				{"id": "ns-c->ns-missing", "source": "ns-c", "target": "ns-missing"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildElements(tt.nodes)

			if got.Nodes == nil || got.Edges == nil {
				t.Fatalf("expected non-nil Nodes and Edges slices")
			}
			if len(got.Nodes) != len(tt.wantNodes) {
				t.Fatalf("nodes: got %d, want %d", len(got.Nodes), len(tt.wantNodes))
			}
			for i, want := range tt.wantNodes {
				assertData(t, "node", i, got.Nodes[i].Data, want)
			}
			if len(got.Edges) != len(tt.wantEdges) {
				t.Fatalf("edges: got %d, want %d", len(got.Edges), len(tt.wantEdges))
			}
			for i, want := range tt.wantEdges {
				assertData(t, "edge", i, got.Edges[i].Data, want)
			}
		})
	}
}

func assertData(t *testing.T, kind string, index int, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s %d: got data %v, want %v", kind, index, got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Errorf("%s %d: data[%q] = %q, want %q", kind, index, key, got[key], wantValue)
		}
	}
}
