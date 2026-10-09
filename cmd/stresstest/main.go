package main

import (
	_ "embed"
	"fmt"
)

//go:embed tmpl/datacenterresource.yaml
var dcTemplate string

//go:embed tmpl/measurementdevice.yaml
var mdTemplate string

//go:embed tmpl/physicalmeasurement.yaml
var pmTemplate string

func main() {
	fmt.Println("dc", dcTemplate)
	fmt.Println("md", mdTemplate)
	fmt.Println("pm", pmTemplate)
}
