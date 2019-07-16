/*
Copyright 2017 The Kubernetes Authors.

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

package common

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ResourceNeeds maps the type to count of resources types needed
type ResourceNeeds map[string]int

// TypeToResources stores all the leased resources with the same type f
type TypeToResources map[string][]Resource

// ConfigType gather the type of config to be applied by Mason in order to construct the resource
type ConfigType struct {
	// Identifier of the struct this maps back to
	Type string `json:"type,omitempty"`
	// Marshaled JSON content
	Content string `json:"content,omitempty"`
}

// DynamicResourceLifeCycle defines the life cycle of a dynamic resource.
// All Resource of a given type will be constructed using the same configuration
type DynamicResourceLifeCycle struct {
	Type string `json:"type"`
	// Initial state to be created as
	InitialState string `json:"state"`
	// Minimum Number of resources to be use a buffer
	MinCount int `json:"min-count"`
	// Maximum resources expected
	MaxCount int `json:"max-count"`
	// Lifespan of a resource, time after which the resource should be reset.
	LifeSpan *time.Duration `json:"lifespan,omitempty"`
	// Config information about how to create the object
	Config ConfigType `json:"config,omitempty"`
	// Needs define the resource needs to create the object
	Needs ResourceNeeds `json:"needs,omitempty"`
}

// DRLCByName helps sorting ResourcesConfig by name
type DRLCByName []DynamicResourceLifeCycle

func (ut DRLCByName) Len() int           { return len(ut) }
func (ut DRLCByName) Swap(i, j int)      { ut[i], ut[j] = ut[j], ut[i] }
func (ut DRLCByName) Less(i, j int) bool { return ut[i].GetName() < ut[j].GetName() }

// GetName implements the Item interface used for storage
func (res DynamicResourceLifeCycle) GetName() string { return res.Type }

// NewDynamicResourceLifeCycleFromConfig parse the a ResourceEntry into a DynamicResourceLifeCycle
func NewDynamicResourceLifeCycleFromConfig(e ResourceEntry) DynamicResourceLifeCycle {
	var dur *time.Duration
	if e.LifeSpan != nil {
		dur = e.LifeSpan.Duration
	}
	return DynamicResourceLifeCycle{
		Type:         e.Type,
		MaxCount:     e.MaxCount,
		MinCount:     e.MinCount,
		LifeSpan:     dur,
		InitialState: e.State,
		Config:       e.Config,
		Needs:        e.Needs,
	}
}

// Copy returns a copy of the TypeToResources
func (t TypeToResources) Copy() TypeToResources {
	n := TypeToResources{}
	for k, v := range t {
		n[k] = v
	}
	return n
}

// ItemToDynamicResourceLifeCycle casts a Item back to a Resource
func ItemToDynamicResourceLifeCycle(i Item) (DynamicResourceLifeCycle, error) {
	res, ok := i.(DynamicResourceLifeCycle)
	if !ok {
		return DynamicResourceLifeCycle{}, fmt.Errorf("cannot construct Resource from received object %v", i)
	}
	return res, nil
}

// GenerateDynamicResourceName generates a unique name for dynamic resources
func GenerateDynamicResourceName() string {
	return uuid.New().String()
}
