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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
)

// Storage is used to decouple ranch functionality with the resource persistence layer
type Storage struct {
	ctx           context.Context
	client        ctrlruntimeclient.Client
	namespace     string
	resourcesLock sync.RWMutex

	// For testing
	now          func() time.Time
	generateName func() string
}

// NewTestingStorage is used only for testing.
func NewTestingStorage(client ctrlruntimeclient.Client, namespace string, updateTime func() time.Time) *Storage {
	return &Storage{
		client:    client,
		namespace: namespace,
		now:       updateTime,
	}
}

// NewStorage instantiates a new Storage with a PersistenceLayer implementation
// If storage string is not empty, it will read resource data from the file
func NewStorage(client ctrlruntimeclient.Client, namespace, storage string) (*Storage, error) {
	s := &Storage{
		client:       client,
		namespace:    namespace,
		now:          func() time.Time { return time.Now() },
		generateName: common.GenerateDynamicResourceName,
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
	o := &crds.ResourceObject{}
	o.FromItem(resource)
	o.Namespace = s.namespace
	return s.client.Create(s.ctx, o)
}

// DeleteResource deletes a resource if it exists, errors otherwise
func (s *Storage) DeleteResource(name string) error {
	o := &crds.ResourceObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
	}
	return s.client.Delete(s.ctx, o)
}

// UpdateResource updates a resource if it exists, errors otherwise
func (s *Storage) UpdateResource(resource common.Resource) (common.Resource, error) {
	resource.LastUpdate = s.now()

	o := &crds.ResourceObject{}
	name := types.NamespacedName{Namespace: s.namespace, Name: resource.GetName()}
	if err := s.client.Get(s.ctx, name, o); err != nil {
		return common.Resource{}, fmt.Errorf("failed to get resource %s before patching it: %v", resource.GetName(), err)
	}

	o.FromItem(resource)
	if err := s.client.Update(s.ctx, o); err != nil {
		return common.Resource{}, fmt.Errorf("failed to update resources %s after patching it: %v", resource.GetName(), err)
	}

	return common.ItemToResource(o.ToItem())
}

// GetResource gets an existing resource, errors otherwise
func (s *Storage) GetResource(name string) (common.Resource, error) {
	o := &crds.ResourceObject{}
	nn := types.NamespacedName{Namespace: s.namespace, Name: name}
	if err := s.client.Get(s.ctx, nn, o); err != nil {
		return common.Resource{}, fmt.Errorf("failed to get resource %s: %v", name, err)
	}

	return common.ItemToResource(o.ToItem())
}

// GetResources list all resources
func (s *Storage) GetResources() ([]common.Resource, error) {
	resourceList := &crds.ResourceObjectList{}
	if err := s.client.List(s.ctx, resourceList, ctrlruntimeclient.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("failed to list resources; %v", err)
	}

	var resources []common.Resource
	for _, resource := range resourceList.Items {
		res, err := common.ItemToResource(resource.ToItem())
		if err != nil {
			return nil, fmt.Errorf("failed to convert item %s to resource: %v", resource.Name, err)
		}
		resources = append(resources, res)
	}

	sort.Stable(common.ResourceByUpdateTime(resources))
	return resources, nil
}

// AddDynamicResourceLifeCycle adds a new dynamic resource life cycle
func (s *Storage) AddDynamicResourceLifeCycle(resource common.DynamicResourceLifeCycle) error {
	o := &crds.DRLCObject{}
	o.FromItem(resource)
	o.Namespace = s.namespace
	return s.client.Create(s.ctx, o)
}

// DeleteDynamicResourceLifeCycle deletes a dynamic resource life cycle if it exists, errors otherwise
func (s *Storage) DeleteDynamicResourceLifeCycle(name string) error {
	o := &crds.DRLCObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
	}
	return s.client.Delete(s.ctx, o)
}

// UpdateDynamicResourceLifeCycle updates a dynamic resource life cycle. if it exists, errors otherwise
func (s *Storage) UpdateDynamicResourceLifeCycle(resource common.DynamicResourceLifeCycle) (common.DynamicResourceLifeCycle, error) {
	dlrc := &crds.DRLCObject{}
	name := types.NamespacedName{Namespace: s.namespace, Name: resource.GetName()}
	if err := s.client.Get(s.ctx, name, dlrc); err != nil {
		return common.DynamicResourceLifeCycle{}, fmt.Errorf("failed to get dlrc %s before patching it: %v", resource.GetName(), err)
	}

	dlrc.FromItem(resource)
	if err := s.client.Update(s.ctx, dlrc); err != nil {
		return common.DynamicResourceLifeCycle{}, fmt.Errorf("failed to update dlrc %s after patching it: %v", resource.GetName(), err)
	}

	return common.ItemToDynamicResourceLifeCycle(dlrc.ToItem())
}

// GetDynamicResourceLifeCycle gets an existing dynamic resource life cycle, errors otherwise
func (s *Storage) GetDynamicResourceLifeCycle(name string) (common.DynamicResourceLifeCycle, error) {
	dlrc := &crds.DRLCObject{}
	nn := types.NamespacedName{Namespace: s.namespace, Name: name}
	if err := s.client.Get(s.ctx, nn, dlrc); err != nil {
		return common.DynamicResourceLifeCycle{}, fmt.Errorf("failed to get dlrc %s: %q", name, err)
	}

	return common.ItemToDynamicResourceLifeCycle(dlrc.ToItem())
}

// GetDynamicResourceLifeCycles list all dynamic resource life cycle
func (s *Storage) GetDynamicResourceLifeCycles() ([]common.DynamicResourceLifeCycle, error) {
	dlrcList := &crds.DRLCObjectList{}
	if err := s.client.List(s.ctx, dlrcList, ctrlruntimeclient.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("failed to list dlrcs: %v", err)
	}

	var resources []common.DynamicResourceLifeCycle
	for _, dlrc := range dlrcList.Items {
		res, err := common.ItemToDynamicResourceLifeCycle(dlrc.ToItem())
		if err != nil {
			return nil, fmt.Errorf("failed to convert dlrc %s: %v", dlrc.GetName(), err)
		}
		resources = append(resources, res)
	}

	return resources, nil
}

// SyncResources will update static and dynamic resources.
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

	if err := func() error {
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
			if !lifeCycleTypes[res.Type] {
				existingSRByName[res.Name] = res
			}
		}

		if err := s.syncStaticResources(newSRByName, existingSRByName); err != nil {
			return err
		}
		if err := s.syncDynamicResourceLifeCycles(newDRLCByType, existingDRLCByType); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		return err
	}

	if err := s.UpdateAllDynamicResources(); err != nil {
		return err
	}
	return nil
}

// updateDynamicResources updates dynamic resource based on an existing dynamic resource life cycle.
// It will make sure than MinCount of resource exists, and attempt to delete expired and resources over MaxCount.
// If resources are held by another user than Boskos, they will be deleted in a following cycle.
func (s *Storage) updateDynamicResources(lifecycle common.DynamicResourceLifeCycle, resources []common.Resource) (toAdd, toDelete []common.Resource) {
	var notInUseRes []common.Resource
	tombStoned := 0
	toBeDeleted := 0
	for _, r := range resources {
		if r.IsInUse() {
			// We can only delete resources not in use.
			continue
		}
		if r.State == common.Tombstone {
			// Ready to be fully deleted.
			toDelete = append(toDelete, r)
			tombStoned++
		} else if r.State == common.ToBeDeleted {
			// Already in the process of cleaning up.
			// Don't create new resources yet, but also don't delete additional
			// resources.
			toBeDeleted++
		} else {
			if r.ExpirationDate != nil && s.now().After(*r.ExpirationDate) {
				// Expired. Don't decrement the active count until it's tombstoned,
				// however, as it might be depending on other resources that need
				// to be released first.
				toDelete = append(toDelete, r)
				toBeDeleted++
			} else {
				notInUseRes = append(notInUseRes, r)
			}
		}
	}

	// Tombstoned resources are ready to be fully deleted, so replace them if necessary.
	activeCount := len(resources) - tombStoned
	for i := activeCount; i < lifecycle.MinCount; i++ {
		res := common.NewResourceFromNewDynamicResourceLifeCycle(s.generateName(), &lifecycle, s.now())
		toAdd = append(toAdd, res)
		activeCount++
	}

	// ToBeDeleted resources may take some time to be fully cleaned up.
	// We can temporarily exceed MaxCount while these are being cleaned up,
	// particularly if MaxCount was recently lowered.
	numberOfResToDelete := activeCount - toBeDeleted - lifecycle.MaxCount
	// Sorting to get consistent deletion mechanism (ease testing)
	sort.Stable(sort.Reverse(common.ResourceByName(notInUseRes)))
	for i := 0; i < len(notInUseRes); i++ {
		res := notInUseRes[i]
		if i < numberOfResToDelete {
			toDelete = append(toDelete, res)
		}
	}
	logrus.Infof("DRLC type %s: adding %+v, deleting %+v", lifecycle.Type, toAdd, toDelete)
	return
}

// UpdateAllDynamicResources queries for all existing DynamicResourceLifeCycles
// and dynamic resources and calls updateDynamicResources for each type.
// This ensures that the MinCount and MaxCount parameters are honored, that
// any expired resources are deleted, and that any Tombstoned resources are
// completely removed.
func (s *Storage) UpdateAllDynamicResources() error {
	var resToAdd, resToDelete []common.Resource
	var dRLCToDelete []common.DynamicResourceLifeCycle
	existingDRLCByType := map[string]common.DynamicResourceLifeCycle{}
	existingDRsByType := map[string][]common.Resource{}

	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	resources, err := s.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot find resources")
		return err
	}
	existingDRLC, err := s.GetDynamicResourceLifeCycles()
	if err != nil {
		logrus.WithError(err).Error("cannot find DynamicResourceLifeCycles")
		return err
	}
	for _, dRLC := range existingDRLC {
		existingDRLCByType[dRLC.Type] = dRLC
	}

	// Filter to only look at dynamic resources
	for _, res := range resources {
		if _, ok := existingDRLCByType[res.Type]; ok {
			existingDRsByType[res.Type] = append(existingDRsByType[res.Type], res)
		}
	}

	for resType, dRLC := range existingDRLCByType {
		existingDRs := existingDRsByType[resType]
		toAdd, toDelete := s.updateDynamicResources(dRLC, existingDRs)
		resToAdd = append(resToAdd, toAdd...)
		resToDelete = append(resToDelete, toDelete...)

		if dRLC.MinCount == 0 && dRLC.MaxCount == 0 {
			currentCount := len(existingDRs)
			addCount := len(resToAdd)
			delCount := len(resToDelete)
			if addCount == 0 && (currentCount == 0 || currentCount == delCount) {
				dRLCToDelete = append(dRLCToDelete, dRLC)
			}
		}
	}

	if err := s.persistResources(resToAdd, resToDelete, true); err != nil {
		logrus.WithError(err).Error("failed to persist resources")
		return err
	}

	if len(dRLCToDelete) > 0 {
		if err := s.persistDynamicResourceLifeCycles(nil, nil, dRLCToDelete); err != nil {
			return err
		}
	}

	return nil
}

// syncDynamicResourceLifeCycles compares the new DRLC configuration against
// the current configuration. If a DRLC has been deleted from the new
// configuration, it is updated to indicate that its dynamic resources should
// be removed.
// No dynamic resources are created, deleted, or modified by this function.
func (s *Storage) syncDynamicResourceLifeCycles(newDRLCByType, existingDRLCByType map[string]common.DynamicResourceLifeCycle) error {
	var finalError error
	var dRLCToUpdate, dRLCToAdd []common.DynamicResourceLifeCycle

	for _, existingDRLC := range existingDRLCByType {
		newDRLC, existsInNew := newDRLCByType[existingDRLC.Type]
		if existsInNew {
			if !reflect.DeepEqual(existingDRLC, newDRLC) {
				dRLCToUpdate = append(dRLCToUpdate, newDRLC)
			}
		} else {
			// Mark for deletion of all associated dynamic resources.
			existingDRLC.MinCount = 0
			existingDRLC.MaxCount = 0
			dRLCToUpdate = append(dRLCToUpdate, existingDRLC)
		}
	}

	for _, newDRLC := range newDRLCByType {
		_, exists := existingDRLCByType[newDRLC.Type]
		if !exists {
			dRLCToAdd = append(dRLCToAdd, newDRLC)
		}
	}

	if err := s.persistDynamicResourceLifeCycles(dRLCToUpdate, dRLCToAdd, nil); err != nil {
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
		} else {
			// Mark this DRLC as pending deletion by setting min and max count to zero.
			dRLC.MinCount = 0
			dRLC.MaxCount = 0
			dRLCToUpdate = append(dRLCToUpdate, dRLC)
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
