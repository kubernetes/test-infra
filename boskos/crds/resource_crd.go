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
		ListKind:   reflect.TypeOf(ResourceCollection{}).Name(),
		Singular:   "resource",
		Plural:     "resources",
		Object:     &ResourceObject{},
		Collection: &ResourceCollection{},
	}
)

// NewTestResourceClient creates a fake CRD rest client for common.Resource
func NewTestResourceClient() ClientInterface {
	return newDummyClient(ResourceType)
}

// ResourceObject represents common.ResourceObject. It implements the Object interface.
type ResourceObject struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          ResourceSpec   `json:"spec,omitempty"`
	Status        ResourceStatus `json:"status,omitempty"`
}

// ResourceCollection is the Collection implementation
type ResourceCollection struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []*ResourceObject `json:"items"`
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

// GetName returns a unique identifier for a given resource
func (in *ResourceObject) GetName() string {
	return in.Name
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

func (in *ResourceObject) toResource() common.Resource {
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

// ToItem implements Object interface
func (in *ResourceObject) ToItem() common.Item {
	return in.toResource()
}

func (in *ResourceObject) fromResource(r common.Resource) {
	in.Name = r.Name
	in.Spec.Type = r.Type
	in.Status.Owner = r.Owner
	in.Status.State = r.State
	in.Status.LastUpdate = r.LastUpdate
	in.Status.UserData = r.UserData
	in.Status.ExpirationDate = r.ExpirationDate
}

// FromItem implements Object interface
func (in *ResourceObject) FromItem(i common.Item) {
	r, err := common.ItemToResource(i)
	if err == nil {
		in.fromResource(r)
	}
}

// GetItems implements Collection interface
func (in *ResourceCollection) GetItems() []Object {
	var items []Object
	for _, i := range in.Items {
		items = append(items, i)
	}
	return items
}

// SetItems implements Collection interface
func (in *ResourceCollection) SetItems(objects []Object) {
	var items []*ResourceObject
	for _, b := range objects {
		items = append(items, b.(*ResourceObject))
	}
	in.Items = items
}

func (in *ResourceCollection) deepCopyInto(out *ResourceCollection) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	out.Items = in.Items
}

func (in *ResourceCollection) deepCopy() *ResourceCollection {
	if in == nil {
		return nil
	}
	out := new(ResourceCollection)
	in.deepCopyInto(out)
	return out
}

// DeepCopyObject implements Collection interface
func (in *ResourceCollection) DeepCopyObject() runtime.Object {
	if c := in.deepCopy(); c != nil {
		return c
	}
	return nil
}
