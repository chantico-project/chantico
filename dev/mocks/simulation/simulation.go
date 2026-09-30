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

// Package simulation provides bounded random-walk value generation shared by
// the development mocks, so that simulated readings drift gradually instead of
// jumping randomly between requests.
package simulation

import (
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Bounds describes the shape of a random walk.
type Bounds struct {
	Min      float64
	Max      float64
	Start    float64
	Variance float64 // Maximum change per step, in either direction.
}

// BoundsFromEnv reads the walk configuration from <prefix>_MIN, <prefix>_MAX,
// <prefix>_START and <prefix>_VARIANCE, falling back to the given defaults.
func BoundsFromEnv(prefix string, defaults Bounds) Bounds {
	return Bounds{
		Min:      EnvFloat(prefix+"_MIN", defaults.Min),
		Max:      EnvFloat(prefix+"_MAX", defaults.Max),
		Start:    EnvFloat(prefix+"_START", defaults.Start),
		Variance: EnvFloat(prefix+"_VARIANCE", defaults.Variance),
	}
}

// Walk generates gradually drifting values, keyed by an arbitrary identifier
// such as an OID or a VM ID. It is safe for concurrent use.
type Walk struct {
	bounds Bounds
	mutex  sync.Mutex
	values map[string]float64
}

// NewWalk creates a Walk with the given bounds.
func NewWalk(bounds Bounds) *Walk {
	return &Walk{bounds: bounds, values: map[string]float64{}}
}

// Next advances the value for a key by a bounded random step and returns it.
func (w *Walk) Next(key string) float64 {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	value, ok := w.values[key]
	if !ok {
		value = w.bounds.Start
	}

	value += (rand.Float64()*2 - 1) * w.bounds.Variance
	value = max(w.bounds.Min, min(w.bounds.Max, value))
	w.values[key] = value

	return value
}

// EnvString reads a string environment variable, falling back to a default.
func EnvString(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

// EnvFloat reads a float environment variable, falling back to a default.
func EnvFloat(name string, defaultValue float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		log.Printf("invalid %s=%q; using default %v", name, raw, defaultValue)
		return defaultValue
	}
	return value
}

// EnvInts reads a comma or whitespace separated list of non-negative integers,
// falling back to a default.
func EnvInts(name string, defaultValues []int) []int {
	raw := os.Getenv(name)
	if raw == "" {
		return defaultValues
	}

	fields := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	if len(fields) == 0 {
		log.Printf("invalid %s=%q; using defaults", name, raw)
		return defaultValues
	}

	values := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil || value < 0 {
			log.Printf("invalid %s=%q; using defaults", name, raw)
			return defaultValues
		}
		values = append(values, value)
	}
	return values
}
