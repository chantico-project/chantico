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

type ErrorServiceDefinedOnParent struct {
	InvolvedResource string
}

func (e ErrorServiceDefinedOnParent) Error() string {
	return fmt.Sprintf("service ID must not be defined on %s with children (no leaf node)", e.InvolvedResource)
}

type ErrorEnergyAttributionTemplateNotFound struct {
	InvolvedResource string
}

func (e ErrorEnergyAttributionTemplateNotFound) Error() string {
	return fmt.Sprintf("energy attribution template not found for parent %s", e.InvolvedResource)
}

type ErrorMissingCoefficientTemplateParameter struct {
	InvolvedResource string
	Parameter        string
}

func (e ErrorMissingCoefficientTemplateParameter) Error() string {
	return fmt.Sprintf("missing parameter %s for coefficient template in parent %s", e.Parameter, e.InvolvedResource)
}

type ErrorCoefficientAndCoefficientTemplateSet struct {
	InvolvedResource string
}

func (e ErrorCoefficientAndCoefficientTemplateSet) Error() string {
	return fmt.Sprintf("parent %s has both coefficient and coefficientTemplate set; only one is allowed", e.InvolvedResource)
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

func validateCoefficientTemplate(
	dataCenterResource *chantico.DataCenterResource,
	energyAttributionTemplates []chantico.EnergyAttributionTemplate,
) error {
	// Validate coefficient templates
	for _, parent := range dataCenterResource.Spec.Parents {
		if parent.CoefficientTemplate.Name == "" {
			continue
		}

		var template *chantico.EnergyAttributionTemplate
		for i := range energyAttributionTemplates {
			if energyAttributionTemplates[i].Name == parent.CoefficientTemplate.Name {
				template = &energyAttributionTemplates[i]
				break
			}
		}
		if template == nil {
			return ErrorEnergyAttributionTemplateNotFound{InvolvedResource: parent.CoefficientTemplate.Name}
		}

		for _, parameterName := range template.Spec.Parameters {
			_, ok := parent.CoefficientTemplate.Parameters[parameterName]
			if !ok {
				return ErrorMissingCoefficientTemplateParameter{
					InvolvedResource: parent.Name,
					Parameter:        parameterName,
				}
			}
		}
	}
	return nil
}

func Validate(
	dataCenterResource *chantico.DataCenterResource,
	dataCenterResources []chantico.DataCenterResource,
	physicalMeasurements []chantico.PhysicalMeasurement,
	energyAttributionTemplates []chantico.EnergyAttributionTemplate,
) ([]chantico.DataCenterResource, string, error) {
	// Perform validation of parent for directed acyclic graph
	resourcesMap := make(map[string]chantico.DataCenterResource)
	visitedSet := make(map[string]bool)
	for _, resource := range dataCenterResources {
		if resource.DeletionTimestamp == nil {
			resourcesMap[resource.ObjectMeta.Name] = resource
		}
		if dataCenterResource.Spec.ServiceId != "" && slices.Contains(resource.Spec.ParentNames(), dataCenterResource.Name) {
			return []chantico.DataCenterResource{}, dataCenterResource.Name, ErrorServiceDefinedOnParent{InvolvedResource: dataCenterResource.Name}
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
		if current.Spec.ServiceId != "" {
			return GetFromMap(resourcesMap, queue[0:visited]), queue[visited], ErrorServiceDefinedOnParent{InvolvedResource: queue[visited]}
		}
		if slices.Contains(current.Spec.ParentNames(), dataCenterResource.ObjectMeta.Name) {
			return GetFromMap(resourcesMap, queue[0:visited]), queue[visited], ErrorCycleDetected{InvolvedResource: queue[visited]}
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
		return GetFromMap(resourcesMap, queue[0:visited]), "", ErrorUnknownType{Type: dataCenterResource.Spec.Type}
	}

	// Root nodes (no parents) must have energyMetric set so Prometheus can
	// source their energy timeseries.
	if len(dataCenterResource.Spec.Parents) == 0 && dataCenterResource.Spec.EnergyMetric == "" {
		return GetFromMap(resourcesMap, queue[0:visited]), "", ErrorMissingEnergyMetric{InvolvedResource: dataCenterResource.Name}
	}

	// Validate that only one of coefficient and coefficientTemplate parameters are set if one is set.
	for _, parent := range dataCenterResource.Spec.Parents {
		if parent.Coefficient != "" && parent.CoefficientTemplate.Name != "" {
			return GetFromMap(resourcesMap, queue[0:visited]), "", ErrorCoefficientAndCoefficientTemplateSet{InvolvedResource: parent.Name}
		}
	}

	err := validateCoefficientTemplate(dataCenterResource, energyAttributionTemplates)
	if err != nil {
		return GetFromMap(resourcesMap, queue[0:visited]), "", err
	}

	return GetFromMap(resourcesMap, queue[0:visited]), "", nil
}
