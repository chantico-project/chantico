package datacenterresource

import (
	"fmt"
	"slices"

	chantico "chantico/api/v1alpha1"

	"github.com/prometheus/common/model"
)

const (
	DataCenterResourceTypePDU        = "pdu"
	DataCenterResourceTypeBaremetal  = "baremetal"
	DataCenterResourceTypeVM         = "vm"
	DataCenterResourceTypeKubernetes = "kubernetes"
	DataCenterResourceTypeHeat       = "heat"
)

type ErrorResourceNotFound struct {
	InvolvedResource string
}

func (e ErrorResourceNotFound) Error() string {
	return fmt.Sprintf("could not locate resource: %s", e.InvolvedResource)
}

type ErrorCycleDetected struct {
	InvolvedResource string
}

func (e ErrorCycleDetected) Error() string {
	return fmt.Sprintf("cyclic loop detected in data center resources from child %s", e.InvolvedResource)
}

type ErrorUnknownType struct {
	Type string
}

func (e ErrorUnknownType) Error() string {
	return fmt.Sprintf("unknown type: %s", e.Type)
}

type ErrorServiceDefinedOnParent struct {
	InvolvedResource string
}

func (e ErrorServiceDefinedOnParent) Error() string {
	return fmt.Sprintf("service ID must not be defined on %s with children (no leaf node)", e.InvolvedResource)
}

type ErrorInvalidLabelName struct {
	LabelName string
}

func (e ErrorInvalidLabelName) Error() string {
	return fmt.Sprintf("invalid additional label name: %s", e.LabelName)
}

func GetFromMap(
	resourcesMap map[string]chantico.DataCenterResource,
	nodes []string,
) []chantico.DataCenterResource {
	result := make([]chantico.DataCenterResource, len(nodes))
	for index, node := range nodes {
		result[index] = resourcesMap[node]
	}
	return result
}

func FormatResources(resources []chantico.DataCenterResource) string {
	text := ""
	for index, resource := range resources {
		if index == 0 {
			text = resource.ObjectMeta.Name
		} else {
			text = fmt.Sprintf("%s, %s", text, resource.ObjectMeta.Name)
		}
	}
	return text
}

// ValidateGraph checks that adding dataCenterResource keeps the parent
// relations a valid directed acyclic graph, with service IDs only on leaf
// nodes.
func validateGraph(
	dataCenterResource *chantico.DataCenterResource,
	dataCenterResources []chantico.DataCenterResource,
	physicalMeasurements []chantico.PhysicalMeasurement,
) ([]chantico.DataCenterResource, string, error) {
	resourcesMap := make(map[string]chantico.DataCenterResource)
	visitedSet := make(map[string]bool)
	for _, resource := range dataCenterResources {
		if resource.DeletionTimestamp == nil {
			resourcesMap[resource.ObjectMeta.Name] = resource
		}
	}
	queue := make([]string, 0)
	queue = append(queue, dataCenterResource.Spec.ParentNames()...)
	visited := 0
	for len(queue) > visited {
		if visitedSet[queue[visited]] {
			queue = append(queue[0:visited], queue[visited+1:]...)
			continue
		}
		current, ok := resourcesMap[queue[visited]]
		if !ok {
			return GetFromMap(resourcesMap, queue[0:visited]), queue[visited], ErrorResourceNotFound{InvolvedResource: queue[visited]}
		}
		if slices.Contains(current.Spec.ParentNames(), dataCenterResource.ObjectMeta.Name) {
			return GetFromMap(resourcesMap, queue[0:visited]), queue[visited], ErrorCycleDetected{InvolvedResource: queue[visited]}
		}
		visitedSet[queue[visited]] = true
		visited = visited + 1
		queue = append(queue, current.Spec.ParentNames()...)
	}

	return GetFromMap(resourcesMap, queue[0:visited]), "", nil
}

// Ensure the resource type is in the list of known types.
func validateResourceType(dataCenterResource *chantico.DataCenterResource) error {
	switch dataCenterResource.Spec.Type {
	case "", DataCenterResourceTypePDU, DataCenterResourceTypeBaremetal, DataCenterResourceTypeVM, DataCenterResourceTypeKubernetes, DataCenterResourceTypeHeat:
		return nil
	default:
		return ErrorUnknownType{Type: dataCenterResource.Spec.Type}
	}
}

// ValidateAdditionalLabelNames rejects additional labels that are not valid Prometheus label names.
func validateAdditionalLabelNames(dataCenterResource *chantico.DataCenterResource) error {
	for labelName := range dataCenterResource.Spec.AdditionalLabels {
		if !model.UTF8Validation.IsValidLabelName(labelName) {
			return ErrorInvalidLabelName{LabelName: labelName}
		}
	}
	return nil
}

var validators = []func(*chantico.DataCenterResource) error{
	validateResourceType,
	validateAdditionalLabelNames,
}

func Validate(
	dataCenterResource *chantico.DataCenterResource,
	dataCenterResources []chantico.DataCenterResource,
	physicalMeasurements []chantico.PhysicalMeasurement,
) ([]chantico.DataCenterResource, string, error) {
	involvedResources, involvedResourceName, err := validateGraph(dataCenterResource, dataCenterResources, physicalMeasurements)
	if err != nil {
		return involvedResources, involvedResourceName, err
	}

	// Check if physical measurements exist
	// TODO(user): For now this validation is skipped because we do not know which
	// order the resources are created

	for _, validate := range validators {
		if err := validate(dataCenterResource); err != nil {
			return involvedResources, "", err
		}
	}

	return involvedResources, "", nil
}
