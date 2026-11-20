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

func TestNeighbourhood(t *testing.T) {
	root := &Node{Name: "ns-root", Label: "root"}
	middle := &Node{Name: "ns-middle", Label: "middle", Parents: []*Node{{Name: "ns-root"}}}
	leaf := &Node{Name: "ns-leaf", Label: "leaf", Parents: []*Node{{Name: "ns-middle"}}}
	isolated := &Node{Name: "ns-isolated", Label: "isolated"}
	nodes := []*Node{root, middle, leaf, isolated}

	tests := []struct {
		name string
		id   string
		want []string
	}{
		{name: "middle node keeps all ancestors and descendants", id: "ns-middle", want: []string{"ns-root", "ns-middle", "ns-leaf"}},
		{name: "root keeps all descendants", id: "ns-root", want: []string{"ns-root", "ns-middle", "ns-leaf"}},
		{name: "leaf keeps all ancestors", id: "ns-leaf", want: []string{"ns-root", "ns-middle", "ns-leaf"}},
		{name: "isolated node stands alone", id: "ns-isolated", want: []string{"ns-isolated"}},
		{name: "unknown node", id: "ns-missing", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Neighbourhood(nodes, tt.id)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %d nodes, want nil", len(got))
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d nodes, want %d", len(got), len(tt.want))
			}
			for i, node := range got {
				if node.Name != tt.want[i] {
					t.Errorf("node %d = %q, want %q", i, node.Name, tt.want[i])
				}
			}
		})
	}
}

func TestNeighbourhoodTopologies(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []*Node
		id        string
		wantNodes []string
		wantEdges []string
	}{
		{
			name: "diamond includes both branches and the shared root",
			nodes: []*Node{
				{Name: "ns-top"},
				{Name: "ns-left", Parents: []*Node{{Name: "ns-top"}}},
				{Name: "ns-right", Parents: []*Node{{Name: "ns-top"}}},
				{Name: "ns-bottom", Parents: []*Node{{Name: "ns-left"}, {Name: "ns-right"}}},
			},
			id:        "ns-left",
			wantNodes: []string{"ns-top", "ns-left", "ns-bottom"},
			wantEdges: []string{"ns-left->ns-top", "ns-bottom->ns-left"},
		},
		{
			name: "co-parent outside the closure is dropped",
			nodes: []*Node{
				{Name: "ns-a"},
				{Name: "ns-b"},
				{Name: "ns-child", Parents: []*Node{{Name: "ns-a"}, {Name: "ns-b"}}},
			},
			id:        "ns-a",
			wantNodes: []string{"ns-a", "ns-child"},
			wantEdges: []string{"ns-child->ns-a"},
		},
		{
			name: "cycle terminates and keeps every member",
			nodes: []*Node{
				{Name: "ns-x", Parents: []*Node{{Name: "ns-y"}}},
				{Name: "ns-y", Parents: []*Node{{Name: "ns-x"}}},
			},
			id:        "ns-x",
			wantNodes: []string{"ns-x", "ns-y"},
			wantEdges: []string{"ns-x->ns-y", "ns-y->ns-x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Neighbourhood(tt.nodes, tt.id)
			var gotNodes []string
			for _, node := range got {
				gotNodes = append(gotNodes, node.Name)
			}
			if !equalStrings(gotNodes, tt.wantNodes) {
				t.Fatalf("nodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			var gotEdges []string
			for _, edge := range BuildElements(got).Edges {
				gotEdges = append(gotEdges, edge.Data["id"])
			}
			if !equalStrings(gotEdges, tt.wantEdges) {
				t.Fatalf("edges = %v, want %v", gotEdges, tt.wantEdges)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
