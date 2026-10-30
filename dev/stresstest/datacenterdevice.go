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
}

func NewDataCenterResourceBuilder() *DataCenterResourceBuilder {
	return &DataCenterResourceBuilder{
		Parents:             []chapi.ParentRef{},
		EnergyMetric:        "",
		Physicalmeasurement: "",
		Name:                "",
		Type:                "",
	}
}

func (d *DataCenterResourceBuilder) AddEnergyMetric(energyMetric, physicalMeasurement string) *DataCenterResourceBuilder {
	d.EnergyMetric = energyMetric
	d.Physicalmeasurement = physicalMeasurement
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

func (d *DataCenterResourceBuilder) Build() chapi.DataCenterResource {
	dcr := chapi.DataCenterResource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DataCenterResource",
			APIVersion: "chantico/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.Name,
			Namespace: "chantico",
		},
	}

	dcr.Spec = chapi.DataCenterResourceSpec{
		Type:                 d.Type,
		PhysicalMeasurements: []string{d.Physicalmeasurement},
		Parents:              d.Parents,
		EnergyMetric:         d.EnergyMetric,
	}
	return dcr
}

func generateDcRootLayer(mdPmCombination []MeasurementDevicePhysicalMeasurementCombination) []chapi.DataCenterResource {
	dcrs := make([]chapi.DataCenterResource, 0, len(mdPmCombination))
	for idx, mdPm := range mdPmCombination {
		walks := mdPm.MeasurementDevice.Spec.Walks
		dcr := NewDataCenterResourceBuilder().
			AddEnergyMetric(walks[rand.Intn(len(walks))], mdPm.PhysicalMeasurements.Name).
			SetName(idx).
			SetType("pdu").
			Build()
		dcrs = append(dcrs, dcr)
	}
	return dcrs
}

func generateDcFirstLayer(rootDCRS []chapi.DataCenterResource, expansionRate int) []chapi.DataCenterResource {
	// Usefull for the new ids
	n := len(rootDCRS)
	m := n * (expansionRate - 1)

	// Initialize dcr builders
	dcrBuilders := make([]DataCenterResourceBuilder, 0, m)
	for i := range m {
		newDcrb := NewDataCenterResourceBuilder().SetName(i + n).SetType("baremetal")
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

func generateDataCenterResources(mdPmCombinations []MeasurementDevicePhysicalMeasurementCombination, expansionRate int) []chapi.DataCenterResource {
	rootDCRS := generateDcRootLayer(mdPmCombinations)
	dcrs := generateDcFirstLayer(rootDCRS, expansionRate)
	return slices.Concat(rootDCRS, dcrs)
}
