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
	"slices"
	"strings"

	chantico "chantico/api/v1alpha1"
)

const (
	DataCenterResourceTypeGrid      = "grid"
	DataCenterResourceTypeSolar     = "solar"
	DataCenterResourceTypeWind      = "wind"
	DataCenterResourceTypeGenerator = "generator"
	DataCenterResourceTypeBattery   = "battery"

	DataCenterResourceTypeSite      = "site"
	DataCenterResourceTypePowerFeed = "power-feed"
	DataCenterResourceTypePDU       = "pdu"
	DataCenterResourceTypeOutlet    = "outlet"

	DataCenterResourceTypeBaremetal  = "baremetal"
	DataCenterResourceTypeHypervisor = "hypervisor"
	DataCenterResourceTypeVM         = "vm"
	DataCenterResourceTypeKubernetes = "kubernetes"
	DataCenterResourceTypeHeat       = "heat"
)

// siteLabel is the label key under which the name of a site is propagated.
const siteLabel = "site"

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
	// PropagatedLabel is the label key under which the name of a resource of
	// this type is applied to the resource itself and all of its descendants,
	// or empty if this type does not propagate a label.
	PropagatedLabel() string
	// Grouping reports whether resources of this type only group other
	// resources, without an energy timeseries of their own. Other resources
	// have an energy metric or parents with coefficients.
	Grouping() bool
}

type typeSpec struct {
	// allowedParents restricts the types of parents; nil accepts any type.
	allowedParents typeSet
	propagate      string
	grouping       bool
}

func (s typeSpec) Grouping() bool {
	return s.grouping
}

func (s typeSpec) AcceptsParent(parentType string) bool {
	if s.allowedParents == nil {
		return true
	}
	_, ok := s.allowedParents[parentType]
	return ok
}

func (s typeSpec) PropagatedLabel() string {
	return s.propagate
}

// Registry of data center resource types and their graph semantics. The keys are
// the values of the spec.type field of the DataCenterResource CRD.
// Keep this table in sync with the allowed parent types table and the site label
// section in docs/content/technical/custom-resources/graph.md.
var typeRegistry = map[string]TypeInfo{
	"":                              typeSpec{},
	DataCenterResourceTypeGrid:      typeSpec{},
	DataCenterResourceTypeSolar:     typeSpec{},
	DataCenterResourceTypeWind:      typeSpec{},
	DataCenterResourceTypeGenerator: typeSpec{},
	DataCenterResourceTypeBattery:   typeSpec{},
	DataCenterResourceTypeSite:      typeSpec{propagate: siteLabel, grouping: true},
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

type ErrorMissingEnergySource struct {
	Type string
}

func (e ErrorMissingEnergySource) Error() string {
	return fmt.Sprintf("resource of type %s must have an energy metric or a parent with a coefficient", e.Type)
}

type ErrorUnexpectedEnergyMetric struct {
	Type string
}

func (e ErrorUnexpectedEnergyMetric) Error() string {
	return fmt.Sprintf("resource of grouping type %s must not have an energy metric", e.Type)
}

type ErrorParentCoefficient struct {
	InvolvedResource string
	ParentType       string
	Grouping         bool
}

func (e ErrorParentCoefficient) Error() string {
	if e.Grouping {
		return fmt.Sprintf("parent %s of grouping type %s must not have a coefficient", e.InvolvedResource, e.ParentType)
	}
	return fmt.Sprintf("parent %s of type %s requires a coefficient", e.InvolvedResource, e.ParentType)
}

type ErrorConflictingLabel struct {
	Label  string
	Values []string
}

func (e ErrorConflictingLabel) Error() string {
	return fmt.Sprintf("multiple ancestors propagate label %s: %s", e.Label, strings.Join(e.Values, ", "))
}

// DeriveLabels collects the labels propagated by the resource itself and by
// its ancestors, according to the type registry. Each label must resolve to a
// single value.
func DeriveLabels(
	dataCenterResource *chantico.DataCenterResource,
	ancestors []chantico.DataCenterResource,
) (map[string]string, error) {
	values := map[string][]string{}
	for _, resource := range append([]chantico.DataCenterResource{*dataCenterResource}, ancestors...) {
		info, err := LookupType(resource.Spec.Type)
		if err != nil {
			continue
		}
		label := info.PropagatedLabel()
		if label == "" {
			continue
		}
		values[label] = append(values[label], resource.Name)
	}
	if len(values) == 0 {
		return nil, nil
	}

	labels := make(map[string]string, len(values))
	for label, names := range values {
		if len(names) > 1 {
			slices.Sort(names)
			return nil, ErrorConflictingLabel{Label: label, Values: names}
		}
		labels[label] = names[0]
	}
	return labels, nil
}
