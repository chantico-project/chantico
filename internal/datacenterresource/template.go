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

func (tr *TemplateRenderer) resolveParentExpressions(ctx context.Context, dataCenterResource *chantico.DataCenterResource) ([]ResolvedParent, error) {
	var parents []ResolvedParent
	for _, parent := range dataCenterResource.Spec.Parents {
		coefficient, err := parent.CoefficientFrom.Render(ctx, tr.client, tr.namespace)
		if err != nil {
			return nil, err
		}
		parents = append(parents, ResolvedParent{
			Name:        parent.Name,
			Coefficient: coefficient,
		})
	}
	return parents, nil
}

func (tr *TemplateRenderer) resolveEnergyMetric(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (string, error) {
	energyMetricFrom := dataCenterResource.Spec.EnergyMetricFrom
	if energyMetricFrom == nil {
		return "", nil
	}
	return energyMetricFrom.Render(ctx, tr.client, tr.namespace)
}

func (tr *TemplateRenderer) ResolvedExpressions(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (*ResolvedExpressions, error) {
	energyMetric, err := tr.resolveEnergyMetric(ctx, dataCenterResource)
	if err != nil {
		return nil, err
	}

	parents, err := tr.resolveParentExpressions(ctx, dataCenterResource)
	if err != nil {
		return nil, err
	}

	return &ResolvedExpressions{
		EnergyMetric: energyMetric,
		Parents:      parents,
	}, nil
}
