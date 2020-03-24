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

package storage

import (
	"sync"

	"fmt"

	"k8s.io/test-infra/boskos/common"
)

// PersistenceLayer defines a simple interface to persists Boskos Information
type PersistenceLayer interface {
	Add(r common.Resource) error
	Delete(name string) error
	Update(r common.Resource) (common.Resource, error)
	Get(name string) (common.Resource, error)
	List() ([]common.Resource, error)
}

type inMemoryStore struct {
	resources map[string]common.Resource
	lock      sync.RWMutex
}

// NewMemoryStorage creates an in memory persistence layer
func NewMemoryStorage() PersistenceLayer {
	return &inMemoryStore{
		resources: map[string]common.Resource{},
	}
}

func (im *inMemoryStore) Add(r common.Resource) error {
	im.lock.Lock()
	defer im.lock.Unlock()
	_, ok := im.resources[r.Name]
	if ok {
		return fmt.Errorf("resource %s already exists", r.Name)
	}
	im.resources[r.Name] = r
	return nil
}

func (im *inMemoryStore) Delete(name string) error {
	im.lock.Lock()
	defer im.lock.Unlock()
	_, ok := im.resources[name]
	if !ok {
		return fmt.Errorf("cannot find item %s", name)
	}
	delete(im.resources, name)
	return nil
}

func (im *inMemoryStore) Update(r common.Resource) (common.Resource, error) {
	im.lock.Lock()
	defer im.lock.Unlock()
	_, ok := im.resources[r.Name]
	if !ok {
		return common.Resource{}, fmt.Errorf("cannot find item %s", r.Name)
	}
	im.resources[r.Name] = r
	return r, nil
}

func (im *inMemoryStore) Get(name string) (common.Resource, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()
	r, ok := im.resources[name]
	if !ok {
		return common.Resource{}, fmt.Errorf("cannot find item %s", name)
	}
	return r, nil
}

func (im *inMemoryStore) List() ([]common.Resource, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()
	var resources []common.Resource
	for _, r := range im.resources {
		resources = append(resources, r)
	}
	return resources, nil
}
