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
	// ResourceType is the ResourceObject CRD type
	ResourceType = Type{
		Kind:       reflect.TypeOf(ResourceObject{}).Name(),
		ListKind:   reflect.TypeOf(ResourceObjectList{}).Name(),
		Singular:   "resource",
		Plural:     "resources",
		Object:     &ResourceObject{},
		Collection: &ResourceObjectList{},
	}
)

// ResourceObject represents common.ResourceObject. It implements the Object interface.
type ResourceObject struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          ResourceSpec   `json:"spec,omitempty"`
	Status        ResourceStatus `json:"status,omitempty"`
}

// ResourceObjectList is the Collection implementation
type ResourceObjectList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []ResourceObject `json:"items"`
}

// ResourceSpec holds information that are not likely to change
type ResourceSpec struct {
	Type string `json:"type"`
}

// ResourceStatus holds information that are likely to change
type ResourceStatus struct {
	State          string           `json:"state,omitempty"`
	Owner          string           `json:"owner"`
	LastUpdate     time.Time        `json:"lastUpdate,omitempty"`
	UserData       *common.UserData `json:"userData,omitempty"`
	ExpirationDate *time.Time       `json:"expirationDate,omitempty"`
}

func (in *ResourceObject) deepCopyInto(out *ResourceObject) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *ResourceObject) deepCopy() *ResourceObject {
	if in == nil {
		return nil
	}
	out := new(ResourceObject)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object interface
func (in *ResourceObject) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}

// ToResource returns the common.Resource representation for
// a ResourceObject
func (in *ResourceObject) ToResource() common.Resource {
	return common.Resource{
		Name:           in.Name,
		Type:           in.Spec.Type,
		Owner:          in.Status.Owner,
		State:          in.Status.State,
		LastUpdate:     in.Status.LastUpdate,
		UserData:       in.Status.UserData,
		ExpirationDate: in.Status.ExpirationDate,
	}
}

// FromResource converts a common.Resource to a *ResourceObject
func FromResource(r common.Resource) *ResourceObject {
	if r.UserData == nil {
		r.UserData = &common.UserData{}
	}
	return &ResourceObject{
		ObjectMeta: v1.ObjectMeta{
			Name: r.Name,
		},
		Spec: ResourceSpec{
			Type: r.Type,
		},
		Status: ResourceStatus{
			Owner:          r.Owner,
			State:          r.State,
			LastUpdate:     r.LastUpdate,
			UserData:       r.UserData,
			ExpirationDate: r.ExpirationDate,
		},
	}
}

func (in *ResourceObjectList) deepCopyInto(out *ResourceObjectList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	out.Items = in.Items
}

func (in *ResourceObjectList) deepCopy() *ResourceObjectList {
	if in == nil {
		return nil
	}
	out := new(ResourceObjectList)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements Collection interface
func (in *ResourceObjectList) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}

// NewResource creates a new Boskos Resource.
func NewResource(name, rtype, state, owner string, t time.Time) *ResourceObject {
	// If no state defined, mark as Free
	if state == "" {
		state = common.Free
	}

	return &ResourceObject{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: ResourceSpec{
			Type: rtype,
		},
		Status: ResourceStatus{
			State:      state,
			Owner:      owner,
			LastUpdate: t,
			UserData:   &common.UserData{},
		},
	}
}
