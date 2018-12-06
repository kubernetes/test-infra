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

package crds

import (
	"reflect"

	"k8s.io/test-infra/boskos/common"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// ResourcesConfigType is the ResourceObject CRD type
	ResourcesConfigType = Type{
		Kind:       reflect.TypeOf(ResourcesConfigObject{}).Name(),
		ListKind:   reflect.TypeOf(ResourcesConfigCollection{}).Name(),
		Singular:   "resourcesconfig",
		Plural:     "resourcesconfigs",
		Object:     &ResourcesConfigObject{},
		Collection: &ResourcesConfigCollection{},
	}
)

// NewTestResourceConfigClient creates a fake CRD rest client for common.Resource
func NewTestResourceConfigClient() ClientInterface {
	return newDummyClient(ResourcesConfigType)
}

// ResourcesConfigObject holds generalized configuration information about how the
// resource needs to be created.
// Some Resource might not have a ResourceConfig (Example Project)
type ResourcesConfigObject struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          ResourcesConfigSpec `json:"spec"`
}

// ResourcesConfigSpec holds config implementation specific configuration as well as resource needs
type ResourcesConfigSpec struct {
	Config common.ConfigType    `json:"config"`
	Needs  common.ResourceNeeds `json:"needs"`
}

// ResourcesConfigCollection implements the Collections interface
type ResourcesConfigCollection struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []*ResourcesConfigObject `json:"items"`
}

// GetName implements the Object interface
func (in *ResourcesConfigObject) GetName() string {
	return in.Name
}

func (in *ResourcesConfigObject) deepCopyInto(out *ResourcesConfigObject) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
}

func (in *ResourcesConfigObject) deepCopy() *ResourcesConfigObject {
	if in == nil {
		return nil
	}
	out := new(ResourcesConfigObject)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface
func (in *ResourcesConfigObject) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ResourcesConfigObject) toConfig() common.ResourcesConfig {
	return common.ResourcesConfig{
		Name:   in.Name,
		Config: in.Spec.Config,
		Needs:  in.Spec.Needs,
	}
}

// ToItem implements the Object interface
func (in *ResourcesConfigObject) ToItem() common.Item {
	return in.toConfig()
}

func (in *ResourcesConfigObject) fromConfig(r common.ResourcesConfig) {
	in.ObjectMeta.Name = r.Name
	in.Spec.Config = r.Config
	in.Spec.Needs = r.Needs
}

// FromItem implements the Object interface
func (in *ResourcesConfigObject) FromItem(i common.Item) {
	c, err := common.ItemToResourcesConfig(i)
	if err == nil {
		in.fromConfig(c)
	}
}

// GetItems implements the Collection interface
func (in *ResourcesConfigCollection) GetItems() []Object {
	var items []Object
	for _, i := range in.Items {
		items = append(items, i)
	}
	return items
}

// SetItems implements the Collection interface
func (in *ResourcesConfigCollection) SetItems(objects []Object) {
	var items []*ResourcesConfigObject
	for _, b := range objects {
		items = append(items, b.(*ResourcesConfigObject))
	}
	in.Items = items
}

func (in *ResourcesConfigCollection) deepCopyInto(out *ResourcesConfigCollection) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	out.Items = in.Items
}

func (in *ResourcesConfigCollection) deepCopy() *ResourcesConfigCollection {
	if in == nil {
		return nil
	}
	out := new(ResourcesConfigCollection)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface
func (in *ResourcesConfigCollection) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}
