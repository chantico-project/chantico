package datacenterresource

import (
	"fmt"
	"slices"

	chantico "chantico/api/v1alpha1"
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

type ErrorMissingEnergyMetric struct {
	InvolvedResource string
}

func (e ErrorMissingEnergyMetric) Error() string {
	return fmt.Sprintf("root node (no parents) %s must have energyMetric set", e.InvolvedResource)
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

func Validate(
	dataCenterResource *chantico.DataCenterResource,
	dataCenterResources []chantico.DataCenterResource,
	physicalMeasurements []chantico.PhysicalMeasurement,
) ([]chantico.DataCenterResource, error, string) {
	// Perform validation of parent for directed acyclic graph
	resourcesMap := make(map[string]chantico.DataCenterResource)
	visitedSet := make(map[string]bool)
	for _, resource := range dataCenterResources {
		if resource.Status.State != StateDelete {
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
			return GetFromMap(resourcesMap, queue[0:visited]), ErrorResourceNotFound{InvolvedResource: queue[visited]}, queue[visited]
		}
		if slices.Contains(current.Spec.ParentNames(), dataCenterResource.ObjectMeta.Name) {
			return GetFromMap(resourcesMap, queue[0:visited]), ErrorCycleDetected{InvolvedResource: queue[visited]}, queue[visited]
		}
		visitedSet[queue[visited]] = true
		visited = visited + 1
		queue = append(queue, current.Spec.ParentNames()...)
	}

	// Check if physical measurements exist
	// TODO(user): For now this validation is skipped because we do not know which
	// order the resources are created

	// Check type of resource
	switch dataCenterResource.Spec.Type {
	case "", DataCenterResourceTypePDU, DataCenterResourceTypeBaremetal, DataCenterResourceTypeVM, DataCenterResourceTypeKubernetes, DataCenterResourceTypeHeat:
	default:
		return GetFromMap(resourcesMap, queue[0:visited]), ErrorUnknownType{Type: dataCenterResource.Spec.Type}, ""
	}

	// Root nodes (no parents) must have energyMetric set so Prometheus can
	// source their energy timeseries.
	if len(dataCenterResource.Spec.Parents) == 0 && dataCenterResource.Spec.EnergyMetric == "" {
		return GetFromMap(resourcesMap, queue[0:visited]), ErrorMissingEnergyMetric{InvolvedResource: dataCenterResource.ObjectMeta.Name}, ""
	}

	return GetFromMap(resourcesMap, queue[0:visited]), nil, ""
}
