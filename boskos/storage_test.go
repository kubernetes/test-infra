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

package main

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/storage"
)

func createStorages() []storage.PersistenceLayer {

	return []storage.PersistenceLayer{
		crds.NewCRDStorage(crds.NewTestResourceClient()),
		storage.NewMemoryStorage(),
	}
}

func TestAddDelete(t *testing.T) {
	for _, s := range createStorages() {
		var resources []common.Resource
		var err error
		for i := 0; i < 10; i++ {
			resources = append(resources, common.Resource{
				Name: fmt.Sprintf("res-%d", i),
				Type: fmt.Sprintf("type-%d", i),
			})
		}
		sort.Stable(common.ResourceByName(resources))
		for _, res := range resources {
			if err = s.Add(res); err != nil {
				t.Errorf("unable to add %s, %v", res.Name, err)
			}
		}
		items, err := s.List()
		if err != nil {
			t.Errorf("unable to to list resources, %v", err)
		}
		var rResources []common.Resource
		for _, i := range items {
			var r common.Resource
			r, err = common.ItemToResource(i)
			if err != nil {
				t.Errorf("unable to convert resource, %v", err)
			}
			rResources = append(rResources, r)
		}
		sort.Stable(common.ResourceByName(rResources))
		if !reflect.DeepEqual(resources, rResources) {
			t.Errorf("received resources (%v) do not match resources (%v)", resources, rResources)
		}
		for _, i := range items {
			err = s.Delete(i.GetName())
			if err != nil {
				t.Errorf("unable to delete resource %s.%v", i.GetName(), err)
			}
		}
		eResources, err := s.List()
		if err != nil {
			t.Errorf("unable to to list resources, %v", err)
		}
		if len(eResources) != 0 {
			t.Error("list should return an empty list")
		}
	}
}

func TestUpdateGet(t *testing.T) {
	for _, s := range createStorages() {
		oRes := common.Resource{
			Name: "original",
			Type: "type",
		}
		if err := s.Add(oRes); err != nil {
			t.Errorf("unable to add resource, %v", err)
		}
		uRes := oRes
		uRes.Type = "typeUpdated"
		if err := s.Update(uRes); err != nil {
			t.Errorf("unable to update resource %v", err)
		}
		i, err := s.Get(oRes.Name)
		if err != nil {
			t.Errorf("unable to get resource, %v", err)
		}
		res, err := common.ItemToResource(i)
		if err != nil {
			t.Errorf("unable to convert resource, %v", err)
		}
		if !reflect.DeepEqual(uRes, res) {
			t.Errorf("expected (%v) and received (%v) do not match", uRes, res)
		}
	}
}

func TestNegativeDeleteGet(t *testing.T) {
	for _, s := range createStorages() {
		oRes := common.Resource{
			Name: "original",
			Type: "type",
		}
		if err := s.Add(oRes); err != nil {
			t.Errorf("unable to add resource, %v", err)
		}
		uRes := common.Resource{
			Name: "notExist",
			Type: "type",
		}
		if err := s.Update(uRes); err == nil {
			t.Errorf("should not be able to update resource, %v", err)
		}
		if err := s.Delete(uRes.Name); err == nil {
			t.Errorf("should not be able to delete resource, %v", err)
		}
	}
}
