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
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/storage"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	deleteGracePeriodSeconds = 10
)

type inClusterStorage struct {
	client ClientInterface
}

// NewCRDStorage creates a Custom Resource Definition persistence layer
func NewCRDStorage(client ClientInterface) storage.PersistenceLayer {
	return &inClusterStorage{
		client: client,
	}
}

func (cs *inClusterStorage) Add(i common.Item) error {
	o := cs.client.NewObject()
	o.FromItem(i)
	_, err := cs.client.Create(o)
	return err
}

func (cs *inClusterStorage) Delete(name string) error {
	return cs.client.Delete(name, v1.NewDeleteOptions(deleteGracePeriodSeconds))
}

func (cs *inClusterStorage) Update(i common.Item) (common.Item, error) {
	o, err := cs.client.Get(i.GetName())
	if err != nil {
		return nil, err
	}
	o.FromItem(i)
	updated, err := cs.client.Update(o)
	if err != nil {
		return nil, err
	}
	return updated.ToItem(), nil
}

func (cs *inClusterStorage) Get(name string) (common.Item, error) {
	o, err := cs.client.Get(name)
	if err != nil {
		return nil, err
	}
	return o.ToItem(), nil
}

func (cs *inClusterStorage) List() ([]common.Item, error) {
	col, err := cs.client.List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var items []common.Item
	for _, i := range col.GetItems() {
		items = append(items, i.ToItem())
	}
	return items, nil
}
