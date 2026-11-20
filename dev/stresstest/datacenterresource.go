package main

import (
	chapi "chantico/api/v1alpha1"
	"fmt"
	"math/rand"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DataCenterResourceBuilder struct {
	Parents             []chapi.ParentRef
	EnergyMetric        string
	Physicalmeasurement string
	Name                string
	Type                string
	Namespace           string
}

func NewDataCenterResourceBuilder() *DataCenterResourceBuilder {
	return &DataCenterResourceBuilder{
		Parents:             []chapi.ParentRef{},
		EnergyMetric:        "",
		Physicalmeasurement: "",
		Name:                "",
		Type:                "",
		Namespace:           "",
	}
}

func (d *DataCenterResourceBuilder) AddEnergyMetric(energyMetric string) *DataCenterResourceBuilder {
	d.EnergyMetric = energyMetric
	return d
}

func (d *DataCenterResourceBuilder) AddParent(parent chapi.ParentRef) *DataCenterResourceBuilder {
	d.Parents = append(d.Parents, parent)
	return d
}

func (d *DataCenterResourceBuilder) SetName(idx int) *DataCenterResourceBuilder {
	d.Name = fmt.Sprintf("stresstest-dcr-%d-%d", time.Now().Unix(), idx)
	return d
}

func (d *DataCenterResourceBuilder) SetType(typ string) *DataCenterResourceBuilder {
	d.Type = typ
	return d
}

func (d *DataCenterResourceBuilder) SetNamespace(namespace string) *DataCenterResourceBuilder {
	d.Namespace = namespace
	return d
}

func (d *DataCenterResourceBuilder) Build() chapi.DataCenterResource {
	dcr := chapi.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.Name,
			Namespace: d.Namespace,
		},
	}

	dcr.Spec = chapi.DataCenterResourceSpec{
		Type:         d.Type,
		Parents:      d.Parents,
		EnergyMetric: d.EnergyMetric,
	}
	return dcr
}

func generateDcRootLayer(
	namespace string,
	mdPmCombination []MeasurementDevicePhysicalMeasurementCombination,
) []chapi.DataCenterResource {
	dcrs := make([]chapi.DataCenterResource, 0, len(mdPmCombination))
	for idx, mdPm := range mdPmCombination {
		walks := mdPm.MeasurementDevice.Spec.Walks
		dcr := NewDataCenterResourceBuilder().
			AddEnergyMetric(walks[rand.Intn(len(walks))]).
			SetName(idx).
			SetType("pdu").
			SetNamespace(namespace).
			Build()
		dcrs = append(dcrs, dcr)
	}
	return dcrs
}

func generateDcFirstLayer(
	namespace string,
	rootDCRS []chapi.DataCenterResource,
	expansionRate int,
) []chapi.DataCenterResource {
	// Useful for the new ids
	n := len(rootDCRS)
	m := n * (expansionRate - 1)

	// Initialize dcr builders
	dcrBuilders := make([]DataCenterResourceBuilder, 0, m)
	for i := range m {
		newDcrb := NewDataCenterResourceBuilder().
			SetName(i + n).
			SetNamespace(namespace).
			SetType("baremetal")
		dcrBuilders = append(dcrBuilders, *newDcrb)
	}

	// Do the parent assignment
	childId := 0
	for _, dcr := range rootDCRS {
		pref := chapi.ParentRef{
			Name:        dcr.Name,
			Coefficient: fmt.Sprintf("%f", 1/float64(expansionRate-1)),
		}
		for range expansionRate {
			dcrBuilders[childId] = *dcrBuilders[childId].AddParent(pref)
			childId = (childId + 1) % m
		}
	}

	// Build the dcr
	dcrs := make([]chapi.DataCenterResource, 0, m)
	for _, dcrb := range dcrBuilders {
		dcrs = append(dcrs, dcrb.Build())
	}

	return dcrs
}

func generateDataCenterResources(
	namespace string,
	mdPmCombinations []MeasurementDevicePhysicalMeasurementCombination,
	expansionRate int,
) []chapi.DataCenterResource {
	rootDCRS := generateDcRootLayer(namespace, mdPmCombinations)
	dcrs := generateDcFirstLayer(namespace, rootDCRS, expansionRate)
	return slices.Concat(rootDCRS, dcrs)
}
