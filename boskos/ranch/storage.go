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
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/storage"
)

// Storage is used to decouple ranch functionality with the resource persistence layer
type Storage struct {
	resources, dynamicResourceLifeCycles storage.PersistenceLayer
	resourcesLock                        sync.RWMutex

	// For testing
	now          func() time.Time
	generateName func() string
}

// NewTestingStorage is used only for testing.
func NewTestingStorage(res, lf storage.PersistenceLayer, updateTime func() time.Time) *Storage {
	return &Storage{
		resources:                 res,
		dynamicResourceLifeCycles: lf,
		now:                       updateTime,
	}
}

// NewStorage instantiates a new Storage with a PersistenceLayer implementation
// If storage string is not empty, it will read resource data from the file
func NewStorage(res, lf storage.PersistenceLayer, storage string) (*Storage, error) {
	s := &Storage{
		resources:                 res,
		dynamicResourceLifeCycles: lf,
		now:                       func() time.Time { return time.Now() },
		generateName:              common.GenerateDynamicResourceName,
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

		for _, res := range data.Resources {
			if err := s.AddResource(res); err != nil {
				logrus.WithError(err).Errorf("Failed Adding Resource: %s - %s.", res.Name, res.State)
			}
			logrus.Infof("Successfully Added Resource: %s - %s.", res.Name, res.State)
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
func (s *Storage) UpdateResource(resource common.Resource) (common.Resource, error) {
	resource.LastUpdate = s.now()
	i, err := s.resources.Update(resource)
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

// AddDynamicResourceLifeCycle adds a new dynamic resource life cycle
func (s *Storage) AddDynamicResourceLifeCycle(resource common.DynamicResourceLifeCycle) error {
	return s.dynamicResourceLifeCycles.Add(resource)
}

// DeleteDynamicResourceLifeCycle deletes a dynamic resource life cycle if it exists, errors otherwise
func (s *Storage) DeleteDynamicResourceLifeCycle(name string) error {
	return s.dynamicResourceLifeCycles.Delete(name)
}

// UpdateDynamicResourceLifeCycle updates a dynamic resource life cycle. if it exists, errors otherwise
func (s *Storage) UpdateDynamicResourceLifeCycle(resource common.DynamicResourceLifeCycle) (common.DynamicResourceLifeCycle, error) {
	i, err := s.dynamicResourceLifeCycles.Update(resource)
	if err != nil {
		return common.DynamicResourceLifeCycle{}, err
	}
	var res common.DynamicResourceLifeCycle
	res, err = common.ItemToDynamicResourceLifeCycle(i)
	if err != nil {
		return common.DynamicResourceLifeCycle{}, err
	}
	return res, nil
}

// GetDynamicResourceLifeCycle gets an existing dynamic resource life cycle, errors otherwise
func (s *Storage) GetDynamicResourceLifeCycle(name string) (common.DynamicResourceLifeCycle, error) {
	i, err := s.dynamicResourceLifeCycles.Get(name)
	if err != nil {
		return common.DynamicResourceLifeCycle{}, err
	}
	var res common.DynamicResourceLifeCycle
	res, err = common.ItemToDynamicResourceLifeCycle(i)
	if err != nil {
		return common.DynamicResourceLifeCycle{}, err
	}
	return res, nil
}

// GetDynamicResourceLifeCycles list all dynamic resource life cycle
func (s *Storage) GetDynamicResourceLifeCycles() ([]common.DynamicResourceLifeCycle, error) {
	var resources []common.DynamicResourceLifeCycle
	items, err := s.dynamicResourceLifeCycles.List()
	if err != nil {
		return resources, err
	}
	for _, i := range items {
		var res common.DynamicResourceLifeCycle
		res, err = common.ItemToDynamicResourceLifeCycle(i)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
	}
	return resources, nil
}

// SyncResources will update static and dynamic resources periodically.
// It will add new resources to storage and try to remove newly deleted resources
// from storage.
// If the newly deleted resource is currently held by a user, the deletion will
// yield to next update cycle.
func (s *Storage) SyncResources(config *common.BoskosConfig) error {
	if config == nil {
		return nil
	}

	newSRByName := map[string]common.Resource{}
	existingSRByName := map[string]common.Resource{}
	existingDRByType := map[string][]common.Resource{}
	newDRLCByType := map[string]common.DynamicResourceLifeCycle{}
	existingDRLCByType := map[string]common.DynamicResourceLifeCycle{}

	for _, entry := range config.Resources {
		if entry.IsDRLC() {
			newDRLCByType[entry.Type] = common.NewDynamicResourceLifeCycleFromConfig(entry)
		} else {
			for _, res := range common.NewResourcesFromConfig(entry) {
				newSRByName[res.Name] = res
			}
		}
	}
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	resources, err := s.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot find resources")
		return err
	}
	existingDRLC, err := s.GetDynamicResourceLifeCycles()
	if err != nil {
		logrus.WithError(err).Error("cannot find dynamicResourceLifeCycles")
		return err
	}
	for _, dRLC := range existingDRLC {
		existingDRLCByType[dRLC.Type] = dRLC
	}

	// Split resources between static and dynamic resources
	lifeCycleTypes := map[string]bool{}

	for _, lc := range existingDRLC {
		lifeCycleTypes[lc.Type] = true
	}
	// Considering the migration case from mason resources to dynamic resources.
	// Dynamic resources already exist but they don't have an associated DRLC
	for _, lc := range newDRLCByType {
		lifeCycleTypes[lc.Type] = true
	}

	for _, res := range resources {
		if lifeCycleTypes[res.Type] {
			existingDRByType[res.Type] = append(existingDRByType[res.Type], res)
		} else {
			existingSRByName[res.Name] = res
		}
	}

	if err := s.syncStaticResources(newSRByName, existingSRByName); err != nil {
		return err
	}
	if err := s.syncDynamicResources(newDRLCByType, existingDRLCByType, existingDRByType); err != nil {
		return err
	}

	return nil
}

// updateDynamicResources will update dynamic resource based on existing on a dynamic resource life cycle.
// It will make sure than MinCount of resource exists, and attempt to delete expired and resources over MaxCount.
// If resources are held by another user than Boskos, they will be deleted in a following cycle.
func (s *Storage) updateDynamicResources(lifecycle common.DynamicResourceLifeCycle, resources []common.Resource) (toAdd, toDelete []common.Resource) {
	var notInUseRes []common.Resource
	count := 0
	for _, r := range resources {
		// We can only delete resources not in use
		if !r.IsInUse() {
			// deleting resources already marked for deletion, and not including them in the count.
			if r.State == common.Tombstone || r.State == common.ToBeDeleted {
				// those will be deleted, not counting
				toDelete = append(toDelete, r)
			} else {
				// Those resources will be deleted at next iteration counting
				count++
				// Expired
				if r.ExpirationDate != nil && s.now().After(*r.ExpirationDate) {
					toDelete = append(toDelete, r)
				} else {
					notInUseRes = append(notInUseRes, r)
				}
			}

		} else {
			count++
		}
	}

	for i := count; i < lifecycle.MinCount; i++ {
		res := common.NewResourceFromNewDynamicResourceLifeCycle(s.generateName(), &lifecycle, s.now())
		toAdd = append(toAdd, res)
		count++
	}

	numberOfResToDelete := count - lifecycle.MaxCount
	// Sorting to get consistent deletion mechanism (ease testing)
	sort.Stable(sort.Reverse(common.ResourceByName(notInUseRes)))
	for i := 0; i < len(notInUseRes); i++ {
		res := notInUseRes[i]
		if i < numberOfResToDelete {
			toDelete = append(toDelete, res)
		}
	}
	return
}

func (s *Storage) syncDynamicResources(newDRLCByType, existingDRLCByType map[string]common.DynamicResourceLifeCycle, existingResByType map[string][]common.Resource) error {
	var finalError error
	var resToAdd, resToDelete []common.Resource
	var dRLCToUpdate, dRLCToAdd, dRLCToDelete []common.DynamicResourceLifeCycle

	for _, existingDRLC := range existingDRLCByType {
		newDRLC, exists := newDRLCByType[existingDRLC.Type]
		if exists {
			if !reflect.DeepEqual(existingDRLC, newDRLC) {
				dRLCToUpdate = append(dRLCToUpdate, newDRLC)
			}
			moreToAdd, moreToDelete := s.updateDynamicResources(newDRLC, existingResByType[newDRLC.Type])
			resToAdd = append(resToAdd, moreToAdd...)
			resToDelete = append(resToDelete, moreToDelete...)
		} else {
			dRLCToDelete = append(dRLCToDelete, existingDRLC)
			for _, res := range existingResByType[existingDRLC.Type] {
				resToDelete = append(resToDelete, res)
			}
		}
	}

	for _, newDRLC := range newDRLCByType {
		_, exists := existingDRLCByType[newDRLC.Type]
		if !exists {
			dRLCToAdd = append(dRLCToAdd, newDRLC)
			moreToAdd, moreToDelete := s.updateDynamicResources(newDRLC, existingResByType[newDRLC.Type])
			resToAdd = append(resToAdd, moreToAdd...)
			resToDelete = append(resToDelete, moreToDelete...)
		}
	}

	if err := s.persistResources(resToAdd, resToDelete, true); err != nil {
		finalError = multierror.Append(finalError, err)
	}
	if err := s.persistDynamicResourceLifeCycles(dRLCToUpdate, dRLCToAdd, dRLCToDelete); err != nil {
		finalError = multierror.Append(finalError, err)
	}

	return finalError
}

func (s *Storage) persistResources(resToAdd, resToDelete []common.Resource, dynamic bool) error {
	var finalError error
	for _, r := range resToDelete {
		// If currently busy, yield deletion to later cycles.
		if !r.IsInUse() {
			if dynamic {
				// Only delete resource in tombsone state and mark the other as to deleted
				// This is necessary for dynamic resources that depends on other resources
				// as they need to be released to prevent leak.
				if r.State == common.Tombstone {
					logrus.Infof("Deleting resource %s", r.Name)
					if err := s.DeleteResource(r.Name); err != nil {
						finalError = multierror.Append(finalError, err)
						logrus.WithError(err).Errorf("unable to delete resource %s", r.Name)
					}
				} else if r.State != common.ToBeDeleted {
					r.State = common.ToBeDeleted
					logrus.Infof("Marking resource to be deleted %s", r.Name)
					if _, err := s.UpdateResource(r); err != nil {
						finalError = multierror.Append(finalError, err)
						logrus.WithError(err).Errorf("unable to update resource %s", r.Name)
					}
				}
			} else {
				// Static resources can be deleted right away.
				logrus.Infof("Deleting resource %s", r.Name)
				if err := s.DeleteResource(r.Name); err != nil {
					finalError = multierror.Append(finalError, err)
					logrus.WithError(err).Errorf("unable to delete resource %s", r.Name)
				}
			}
		}
	}

	for _, r := range resToAdd {
		logrus.Infof("Adding resource %s", r.Name)
		r.LastUpdate = s.now()
		if err := s.AddResource(r); err != nil {
			finalError = multierror.Append(finalError, err)
			logrus.WithError(err).Errorf("unable to delete resource %s", r.Name)
		}
	}

	return finalError
}

func (s *Storage) persistDynamicResourceLifeCycles(dRLCToUpdate, dRLCToAdd, dRLCToDelelete []common.DynamicResourceLifeCycle) error {
	var finalError error
	remainingTypes := map[string]bool{}
	updatedResources, err := s.GetResources()
	if err != nil {
		return err
	}
	for _, res := range updatedResources {
		remainingTypes[res.Type] = true
	}

	for _, dRLC := range dRLCToDelelete {
		// Only delete a dynamic resource if all resources are gone
		if !remainingTypes[dRLC.Type] {
			logrus.Infof("Deleting resource type life cycle %s", dRLC.Type)
			if err := s.DeleteDynamicResourceLifeCycle(dRLC.Type); err != nil {
				finalError = multierror.Append(finalError, err)
				logrus.WithError(err).Errorf("unable to delete resource type life cycle %s", dRLC.Type)
			}
		}
	}

	for _, DRLC := range dRLCToAdd {
		logrus.Infof("Adding resource type life cycle %s", DRLC.Type)
		if err := s.AddDynamicResourceLifeCycle(DRLC); err != nil {
			finalError = multierror.Append(finalError, err)
			logrus.WithError(err).Errorf("unable to add resource type life cycle %s", DRLC.Type)
		}
	}

	for _, dRLC := range dRLCToUpdate {
		logrus.Infof("Updating resource type life cycle %s", dRLC.Type)
		if _, err := s.UpdateDynamicResourceLifeCycle(dRLC); err != nil {
			finalError = multierror.Append(finalError, err)
			logrus.WithError(err).Errorf("unable to update resource type life cycle %s", dRLC.Type)
		}
	}

	return finalError
}

func (s *Storage) syncStaticResources(newResourcesByName, existingResourcesByName map[string]common.Resource) error {
	var resToAdd, resToDelete []common.Resource

	// Delete resources
	for _, res := range existingResourcesByName {
		_, exists := newResourcesByName[res.Name]
		if !exists {
			resToDelete = append(resToDelete, res)
		}
	}

	// Add new resources
	for _, res := range newResourcesByName {
		_, exists := existingResourcesByName[res.Name]
		if !exists {
			resToAdd = append(resToAdd, res)
		}
	}
	return s.persistResources(resToAdd, resToDelete, false)
}
