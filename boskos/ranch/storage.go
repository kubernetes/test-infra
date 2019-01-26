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

package ranch

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/storage"
)

// Storage is used to decouple ranch functionality with the resource persistence layer
type Storage struct {
	resources     storage.PersistenceLayer
	resourcesLock sync.RWMutex
}

// NewStorage instantiates a new Storage with a PersistenceLayer implementation
// If storage string is not empty, it will read resource data from the file
func NewStorage(r storage.PersistenceLayer, storage string) (*Storage, error) {
	s := &Storage{
		resources: r,
	}

	if storage != "" {
		var data struct {
			Resources []common.Resource
		}
		buf, err := ioutil.ReadFile(storage)
		if err == nil {
			logrus.Infof("Current state: %s.", string(buf))
			err = json.Unmarshal(buf, &data)
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}

		logrus.Info("Before adding resource loop")
		for _, res := range data.Resources {
			if err := s.AddResource(res); err != nil {
				logrus.WithError(err).Errorf("Failed Adding Resources: %s - %s.", res.Name, res.State)
			}
			logrus.Infof("Successfully Added Resources: %s - %s.", res.Name, res.State)
		}
	}
	return s, nil
}

// AddResource adds a new resource
func (s *Storage) AddResource(resource common.Resource) error {
	return s.resources.Add(resource)
}

// DeleteResource deletes a resource if it exists, errors otherwise
func (s *Storage) DeleteResource(name string) error {
	return s.resources.Delete(name)
}

// UpdateResource updates a resource if it exists, errors otherwise
func (s *Storage) UpdateResource(resource common.Resource) error {
	return s.resources.Update(resource)
}

// GetResource gets an existing resource, errors otherwise
func (s *Storage) GetResource(name string) (common.Resource, error) {
	i, err := s.resources.Get(name)
	if err != nil {
		return common.Resource{}, err
	}
	var res common.Resource
	res, err = common.ItemToResource(i)
	if err != nil {
		return common.Resource{}, err
	}
	return res, nil
}

// GetResources list all resources
func (s *Storage) GetResources() ([]common.Resource, error) {
	var resources []common.Resource
	items, err := s.resources.List()
	if err != nil {
		return resources, err
	}
	for _, i := range items {
		var res common.Resource
		res, err = common.ItemToResource(i)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
	}
	sort.Stable(common.ResourceByUpdateTime(resources))
	return resources, nil
}

// SyncResources will update resources every 10 mins.
// It will append newly added resources to ranch.Resources,
// And try to remove newly deleted resources from ranch.Resources.
// If the newly deleted resource is currently held by a user, the deletion will
// yield to next update cycle.
func (s *Storage) SyncResources(data []common.Resource) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	resources, err := s.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot find resources")
		return err
	}

	var finalError error

	// delete non-exist resource
	valid := 0
	for _, res := range resources {
		// If currently busy, yield deletion to later cycles.
		if res.Owner != "" {
			resources[valid] = res
			valid++
			continue
		}
		toDelete := true
		for _, newRes := range data {
			if res.Name == newRes.Name {
				resources[valid] = res
				valid++
				toDelete = false
				break
			}
		}
		if toDelete {
			logrus.Infof("Deleting resource %s", res.Name)
			if err := s.DeleteResource(res.Name); err != nil {
				finalError = multierror.Append(finalError, err)
				logrus.WithError(err).Errorf("unable to delete resource %s", res.Name)
			}
		}
	}
	resources = resources[:valid]

	// add new resource
	for _, p := range data {
		found := false
		for idx := range resources {
			exist := resources[idx]
			if p.Name == exist.Name {
				found = true
				logrus.Infof("Keeping resource %s", p.Name)
				break
			}
		}

		if !found {
			if p.State == "" {
				p.State = common.Free
			}
			logrus.Infof("Adding resource %s", p.Name)
			resources = append(resources, p)
			if err := s.AddResource(p); err != nil {
				logrus.WithError(err).Errorf("unable to add resource %s", p.Name)
				finalError = multierror.Append(finalError, err)
			}
		}
	}
	return finalError
}

// ParseConfig reads in configPath and returns a list of resource objects
// on success.
func ParseConfig(configPath string) ([]common.Resource, error) {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var data common.BoskosConfig
	err = yaml.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}

	var resources []common.Resource
	for _, entry := range data.Resources {
		resources = append(resources, common.NewResourcesFromConfig(entry)...)
	}
	return resources, nil
}
