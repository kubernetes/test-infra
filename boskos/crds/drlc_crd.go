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
		ListKind:   reflect.TypeOf(DRLCObjectList{}).Name(),
		Singular:   "dynamicresourcelifecycle",
		Plural:     "dynamicresourcelifecycles",
		Object:     &DRLCObject{},
		Collection: &DRLCObjectList{},
	}
)

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

// DRLCObjectList implements the Collections interface
type DRLCObjectList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []DRLCObject `json:"items"`
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

func (in *DRLCObject) ToDynamicResourceLifeCycle() common.DynamicResourceLifeCycle {
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

// FromDynamicResourceLifecycle converts a common.DynamicResourceLifeCycle into a *DRLCObject
func FromDynamicResourceLifecycle(r common.DynamicResourceLifeCycle) *DRLCObject {
	return &DRLCObject{
		ObjectMeta: v1.ObjectMeta{
			Name: r.Type,
		},
		Spec: DRLCSpec{
			InitialState: r.InitialState,
			MinCount:     r.MinCount,
			MaxCount:     r.MaxCount,
			LifeSpan:     r.LifeSpan,
			Config:       r.Config,
			Needs:        r.Needs,
		},
	}
}

func (in *DRLCObjectList) deepCopyInto(out *DRLCObjectList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	out.Items = in.Items
}

func (in *DRLCObjectList) deepCopy() *DRLCObjectList {
	if in == nil {
		return nil
	}
	out := new(DRLCObjectList)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface
func (in *DRLCObjectList) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}
