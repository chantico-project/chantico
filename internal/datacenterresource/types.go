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

package datacenterresource

import (
	"fmt"
)

const (
	DataCenterResourceTypeGrid      = "grid"
	DataCenterResourceTypeSolar     = "solar"
	DataCenterResourceTypeWind      = "wind"
	DataCenterResourceTypeGenerator = "generator"
	DataCenterResourceTypeBattery   = "battery"

	DataCenterResourceTypePowerFeed = "power-feed"
	DataCenterResourceTypePDU       = "pdu"
	DataCenterResourceTypeOutlet    = "outlet"

	DataCenterResourceTypeBaremetal  = "baremetal"
	DataCenterResourceTypeHypervisor = "hypervisor"
	DataCenterResourceTypeVM         = "vm"
	DataCenterResourceTypeKubernetes = "kubernetes"
	DataCenterResourceTypeHeat       = "heat"
)

type typeSet map[string]struct{}

func setOf(types ...string) typeSet {
	set := make(typeSet, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return set
}

// TypeInfo describes the graph semantics of a data center resource type.
type TypeInfo interface {
	// AcceptsParent reports whether a parent of the given type is allowed.
	AcceptsParent(parentType string) bool
}

type typeSpec struct {
	// allowedParents restricts the types of parents; nil accepts any type.
	allowedParents typeSet
}

func (s typeSpec) AcceptsParent(parentType string) bool {
	if s.allowedParents == nil {
		return true
	}
	_, ok := s.allowedParents[parentType]
	return ok
}

// Registry of data center resource types and their graph semantics. The keys are
// the values of the spec.type field of the DataCenterResource CRD.
// Keep this table in sync with the allowed parent types table in
// docs/content/technical/custom-resources/graph.md.
var typeRegistry = map[string]TypeInfo{
	"":                              typeSpec{},
	DataCenterResourceTypeGrid:      typeSpec{},
	DataCenterResourceTypeSolar:     typeSpec{},
	DataCenterResourceTypeWind:      typeSpec{},
	DataCenterResourceTypeGenerator: typeSpec{},
	DataCenterResourceTypeBattery:   typeSpec{},
	DataCenterResourceTypePowerFeed: typeSpec{},
	DataCenterResourceTypePDU:       typeSpec{},
	DataCenterResourceTypeOutlet: typeSpec{
		allowedParents: setOf(DataCenterResourceTypePDU),
	},
	DataCenterResourceTypeBaremetal: typeSpec{
		allowedParents: setOf(DataCenterResourceTypePDU, DataCenterResourceTypeOutlet),
	},
	DataCenterResourceTypeHypervisor: typeSpec{},
	DataCenterResourceTypeVM: typeSpec{
		allowedParents: setOf(DataCenterResourceTypeBaremetal, DataCenterResourceTypeHypervisor),
	},
	DataCenterResourceTypeKubernetes: typeSpec{
		allowedParents: setOf(DataCenterResourceTypeBaremetal, DataCenterResourceTypeVM),
	},
	DataCenterResourceTypeHeat: typeSpec{},
}

// LookupType returns the semantics of the given spec.type value.
func LookupType(resourceType string) (TypeInfo, error) {
	info, ok := typeRegistry[resourceType]
	if !ok {
		return nil, ErrorUnknownType{Type: resourceType}
	}
	return info, nil
}

type ErrorUnknownType struct {
	Type string
}

func (e ErrorUnknownType) Error() string {
	return fmt.Sprintf("unknown type: %s", e.Type)
}

type ErrorInvalidParentType struct {
	InvolvedResource string
	Type             string
	ParentType       string
}

func (e ErrorInvalidParentType) Error() string {
	return fmt.Sprintf("parent %s of type %s is not allowed for type %s", e.InvolvedResource, e.ParentType, e.Type)
}
