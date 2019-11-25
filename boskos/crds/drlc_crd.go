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
	"time"

	"k8s.io/test-infra/boskos/common"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// DRLCType is the DynamicResourceLifeCycle CRD type
	DRLCType = Type{
		Kind:       reflect.TypeOf(DRLCObject{}).Name(),
		ListKind:   reflect.TypeOf(DRLCCollection{}).Name(),
		Singular:   "dynamicresourcelifecycle",
		Plural:     "dynamicresourcelifecycles",
		Object:     &DRLCObject{},
		Collection: &DRLCCollection{},
	}
)

// NewTestDRLCClient creates a fake CRD rest client for common.Resource
func NewTestDRLCClient() ClientInterface {
	return newDummyClient(DRLCType)
}

// DRLCObject holds generalized configuration information about how the
// resource needs to be created.
// Some Resource might not have a ResourcezConfig (Example Project)
type DRLCObject struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          DRLCSpec `json:"spec"`
}

// DRLCSpec holds config implementation specific configuration as well as resource needs
type DRLCSpec struct {
	InitialState string               `json:"state"`
	MaxCount     int                  `json:"max-count"`
	MinCount     int                  `json:"min-count"`
	LifeSpan     *time.Duration       `json:"lifespan,omitempty"`
	Config       common.ConfigType    `json:"config"`
	Needs        common.ResourceNeeds `json:"needs"`
}

// DRLCCollection implements the Collections interface
type DRLCCollection struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []*DRLCObject `json:"items"`
}

// GetName implements the Object interface
func (in *DRLCObject) GetName() string {
	return in.Name
}

func (in *DRLCObject) deepCopyInto(out *DRLCObject) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
}

func (in *DRLCObject) deepCopy() *DRLCObject {
	if in == nil {
		return nil
	}
	out := new(DRLCObject)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface
func (in *DRLCObject) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *DRLCObject) toDynamicResourceLifeCycle() common.DynamicResourceLifeCycle {
	return common.DynamicResourceLifeCycle{
		Type:         in.Name,
		InitialState: in.Spec.InitialState,
		MinCount:     in.Spec.MinCount,
		MaxCount:     in.Spec.MaxCount,
		LifeSpan:     in.Spec.LifeSpan,
		Config:       in.Spec.Config,
		Needs:        in.Spec.Needs,
	}
}

func (in *DRLCObject) fromDynamicResourceLifeCycle(r common.DynamicResourceLifeCycle) {
	in.ObjectMeta.Name = r.Type
	in.Spec.InitialState = r.InitialState
	in.Spec.MinCount = r.MinCount
	in.Spec.MaxCount = r.MaxCount
	in.Spec.LifeSpan = r.LifeSpan
	in.Spec.Config = r.Config
	in.Spec.Needs = r.Needs
}

// ToItem implements the Object interface
func (in *DRLCObject) ToItem() common.Item {
	return in.toDynamicResourceLifeCycle()
}

// FromItem implements the Object interface
func (in *DRLCObject) FromItem(i common.Item) {
	c, err := common.ItemToDynamicResourceLifeCycle(i)
	if err == nil {
		in.fromDynamicResourceLifeCycle(c)
	}
}

// GetItems implements the Collection interface
func (in *DRLCCollection) GetItems() []Object {
	var items []Object
	for _, i := range in.Items {
		items = append(items, i)
	}
	return items
}

// SetItems implements the Collection interface
func (in *DRLCCollection) SetItems(objects []Object) {
	var items []*DRLCObject
	for _, b := range objects {
		items = append(items, b.(*DRLCObject))
	}
	in.Items = items
}

func (in *DRLCCollection) deepCopyInto(out *DRLCCollection) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	out.Items = in.Items
}

func (in *DRLCCollection) deepCopy() *DRLCCollection {
	if in == nil {
		return nil
	}
	out := new(DRLCCollection)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface
func (in *DRLCCollection) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}
