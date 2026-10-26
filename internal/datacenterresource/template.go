/*
Copyright 2025.

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
	chantico "chantico/api/v1alpha1"
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResolvedExpressions struct {
	EnergyMetric string
	Parents      []ResolvedParent
}

type ResolvedParent struct {
	Name        string
	Coefficient string
}

type TemplateRenderer struct {
	client    client.Client
	namespace string
}

func NewTemplateRenderer(client client.Client, namespace string) *TemplateRenderer {
	return &TemplateRenderer{
		client:    client,
		namespace: namespace,
	}
}

func (tr *TemplateRenderer) resolveEnergyMetricTemplate(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (string, error) {
	spec := dataCenterResource.Spec
	if spec.EnergyMetricFrom != nil {
		return spec.EnergyMetricFrom.Render(ctx, tr.client, tr.namespace)
	}
	return spec.EnergyMetric, nil
}

func (tr *TemplateRenderer) resolveParentCoefficientTemplate(ctx context.Context, parent *chantico.ParentRef) (string, error) {
	if parent.CoefficientFrom != nil {
		return parent.CoefficientFrom.Render(ctx, tr.client, tr.namespace)
	}
	if parent.Coefficient != "" {
		return parent.Coefficient, nil
	}
	return "", fmt.Errorf("no coefficient or coefficient template specified for parent %q", parent.Name)
}

func (tr *TemplateRenderer) ResolvedExpressions(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (*ResolvedExpressions, error) {
	energyMetric, err := tr.resolveEnergyMetricTemplate(ctx, dataCenterResource)
	if err != nil {
		return nil, err
	}

	var parents []ResolvedParent
	for _, parent := range dataCenterResource.Spec.Parents {
		coefficient, err := tr.resolveParentCoefficientTemplate(ctx, &parent)
		if err != nil {
			return nil, err
		}
		parents = append(parents, ResolvedParent{
			Name:        parent.Name,
			Coefficient: coefficient,
		})
	}

	return &ResolvedExpressions{
		EnergyMetric: energyMetric,
		Parents:      parents,
	}, nil
}
