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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/boskos/common"
)

// Ranch is the place which all of the Resource objects lives.
type Ranch struct {
	Storage       *Storage
	resourcesLock sync.RWMutex
	requestMgr    *RequestManager
	//
	now func() time.Time
}

func updateTime() time.Time {
	return time.Now()
}

// Public errors:

// ResourceNotFound will be returned if requested resource does not exist.
type ResourceNotFound struct {
	name string
}

func (r ResourceNotFound) Error() string {
	return fmt.Sprintf("no available resource %s, try again later.", r.name)
}

// ResourceTypeNotFound will be returned if requested resource type does not exist.
type ResourceTypeNotFound struct {
	rType string
}

func (r ResourceTypeNotFound) Error() string {
	return fmt.Sprintf("resource type %s does not exist", r.rType)
}

// OwnerNotMatch will be returned if request owner does not match current owner for target resource.
type OwnerNotMatch struct {
	request string
	owner   string
}

func (o OwnerNotMatch) Error() string {
	return fmt.Sprintf("owner mismatch request by %s, currently owned by %s", o.request, o.owner)
}

// StateNotMatch will be returned if requested state does not match current state for target resource.
type StateNotMatch struct {
	expect  string
	current string
}

func (s StateNotMatch) Error() string {
	return fmt.Sprintf("state mismatch - expected %v, current %v", s.expect, s.current)
}

// NewRanch creates a new Ranch object.
// In: config - path to resource file
//     storage - path to where to save/restore the state data
// Out: A Ranch object, loaded from config/storage, or error
func NewRanch(config string, s *Storage, ttl time.Duration) (*Ranch, error) {
	newRanch := &Ranch{
		Storage:    s,
		requestMgr: NewRequestManager(ttl),
		now:        time.Now,
	}
	if config != "" {
		if err := newRanch.SyncConfig(config); err != nil {
			return nil, err
		}
		logrus.Infof("Loaded Boskos configuration successfully")
	}
	return newRanch, nil
}

// acquireRequestPriorityKey is used as key for request priority cache.
type acquireRequestPriorityKey struct {
	rType, state string
}

// Acquire checks out a type of resource in certain state without an owner,
// and move the checked out resource to the end of the resource list.
// In: rtype - name of the target resource
//     state - current state of the requested resource
//     dest - destination state of the requested resource
//     owner - requester of the resource
//     requestID - request ID to get a priority in the queue
// Out: A valid Resource object on success, or
//      ResourceNotFound error if target type resource does not exist in target state.
func (r *Ranch) Acquire(rType, state, dest, owner, requestID string) (*common.Resource, error) {
	logger := logrus.WithFields(logrus.Fields{
		"type":       rType,
		"state":      state,
		"dest":       dest,
		"owner":      owner,
		"identifier": requestID,
	})
	logger.Debug("Acquiring resource lock...")
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	logger.Debug("Determining request priority...")
	ts := acquireRequestPriorityKey{rType: rType, state: state}
	rank, new := r.requestMgr.GetRank(ts, requestID)
	logger.WithFields(logrus.Fields{"rank": rank, "new": new}).Debug("Determined request priority.")

	resources, err := r.Storage.GetResources()
	if err != nil {
		logger.WithError(err).Errorf("could not get resources")
		return nil, &ResourceNotFound{rType}
	}
	logger.Debugf("Considering %d resources.", len(resources))

	// For request priority we need to go over all the list until a matching rank
	matchingResoucesCount := 0
	typeCount := 0
	for idx := range resources {
		res := resources[idx]
		if rType != res.Type {
			continue
		}
		typeCount++

		if state != res.State || res.Owner != "" {
			continue
		}
		matchingResoucesCount++

		if matchingResoucesCount < rank {
			continue
		}
		logger = logger.WithField("resource", res.Name)
		res.Owner = owner
		res.State = dest
		logger.Debug("Updating resource.")
		updatedRes, err := r.Storage.UpdateResource(res)
		if err != nil {
			logger.WithError(err).Errorf("could not update resource %s", res.Name)
			return nil, err
		}
		// Deleting this request since it has been fulfilled
		if requestID != "" {
			logger.Debug("Cleaning up requests.")
			r.requestMgr.Delete(ts, requestID)
		}
		logger.Debug("Successfully acquired resource.")
		return &updatedRes, nil
	}

	if new {
		logger.Debug("Checking for associated dynamic resource type...")
		lifeCycle, err := r.Storage.GetDynamicResourceLifeCycle(rType)
		// Assuming error means no associated dynamic resource
		if err == nil {
			if typeCount < lifeCycle.MaxCount {
				logger.Debug("Adding new dynamic resources...")
				res := common.NewResourceFromNewDynamicResourceLifeCycle(r.Storage.generateName(), &lifeCycle, r.now())
				if err := r.Storage.AddResource(res); err != nil {
					logger.WithError(err).Warningf("unable to add a new resource of type %s", rType)
				}
				logger.Infof("Added dynamic resource %s of type %s", res.Name, res.Type)
			}
		}
	}

	if typeCount > 0 {
		return nil, &ResourceNotFound{rType}
	}
	return nil, &ResourceTypeNotFound{rType}
}

// AcquireByState checks out resources of a given type without an owner,
// that matches a list of resources names.
// In: state - current state of the requested resource
//     dest - destination state of the requested resource
//     owner - requester of the resource
//     names - names of resource to acquire
// Out: A valid list of Resource object on success, or
//      ResourceNotFound error if target type resource does not exist in target state.
func (r *Ranch) AcquireByState(state, dest, owner string, names []string) ([]common.Resource, error) {
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	if names == nil {
		return nil, fmt.Errorf("must provide names of expected resources")
	}

	rNames := sets.NewString(names...)

	allResources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Errorf("could not get resources")
		return nil, &ResourceNotFound{state}
	}

	var resources []common.Resource

	for idx := range allResources {
		res := allResources[idx]
		if state != res.State || res.Owner != "" || !rNames.Has(res.Name) {
			continue
		}

		res.Owner = owner
		res.State = dest
		updatedRes, err := r.Storage.UpdateResource(res)
		if err != nil {
			logrus.WithError(err).Errorf("could not update resource %s", res.Name)
			return nil, err
		}
		resources = append(resources, updatedRes)
		rNames.Delete(res.Name)
	}

	if rNames.Len() != 0 {
		var missingResources []string
		for _, n := range rNames.List() {
			missingResources = append(missingResources, n)
		}
		err := &ResourceNotFound{state}
		logrus.WithError(err).Errorf("could not find required resources %s", strings.Join(missingResources, ", "))
		return resources, err
	}
	return resources, nil
}

// Release unsets owner for target resource and move it to a new state.
// In: name - name of the target resource
//     dest - destination state of the resource
//     owner - owner of the resource
// Out: nil on success, or
//      OwnerNotMatch error if owner does not match current owner of the resource, or
//      ResourceNotFound error if target named resource does not exist.
func (r *Ranch) Release(name, dest, owner string) error {
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	res, err := r.Storage.GetResource(name)
	if err != nil {
		logrus.WithError(err).Errorf("unable to release resource %s", name)
		return &ResourceNotFound{name}
	}
	if owner != res.Owner {
		return &OwnerNotMatch{request: owner, owner: res.Owner}
	}

	res.Owner = ""
	res.State = dest

	if lf, err := r.Storage.GetDynamicResourceLifeCycle(res.Type); err == nil {
		// Assuming error means not existing as the only way to differentiate would be to list
		// all resources and find the right one which is more costly.
		if lf.LifeSpan != nil {
			expirationTime := r.now().Add(*lf.LifeSpan)
			res.ExpirationDate = &expirationTime
		}
	} else {
		res.ExpirationDate = nil
	}

	if _, err := r.Storage.UpdateResource(res); err != nil {
		logrus.WithError(err).Errorf("could not update resource %s", res.Name)
		return err
	}
	return nil
}

// Update updates the timestamp of a target resource.
// In: name  - name of the target resource
//     state - current state of the resource
//     owner - current owner of the resource
// 	   info  - information on how to use the resource
// Out: nil on success, or
//      OwnerNotMatch error if owner does not match current owner of the resource, or
//      ResourceNotFound error if target named resource does not exist, or
//      StateNotMatch error if state does not match current state of the resource.
func (r *Ranch) Update(name, owner, state string, ud *common.UserData) error {
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	res, err := r.Storage.GetResource(name)
	if err != nil {
		logrus.WithError(err).Errorf("could not find resource %s for update", name)
		return &ResourceNotFound{name}
	}
	if owner != res.Owner {
		return &OwnerNotMatch{request: owner, owner: res.Owner}
	}
	if state != res.State {
		return &StateNotMatch{res.State, state}
	}
	if res.UserData == nil {
		res.UserData = &common.UserData{}
	}
	res.UserData.Update(ud)
	if _, err := r.Storage.UpdateResource(res); err != nil {
		logrus.WithError(err).Errorf("could not update resource %s", res.Name)
		return err
	}
	return nil
}

// Reset unstucks a type of stale resource to a new state.
// In: rtype - type of the resource
//     state - current state of the resource
//     expire - duration before resource's last update
//     dest - destination state of expired resources
// Out: map of resource name - resource owner.
func (r *Ranch) Reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	ret := make(map[string]string)

	resources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Errorf("cannot find resources")
		return nil, err
	}

	for idx := range resources {
		res := resources[idx]
		if rtype != res.Type || state != res.State || res.Owner == "" || r.now().Sub(res.LastUpdate) < expire {
			continue
		}

		ret[res.Name] = res.Owner
		res.Owner = ""
		res.State = dest
		if _, err := r.Storage.UpdateResource(res); err != nil {
			logrus.WithError(err).Errorf("could not update resource %s", res.Name)
			return ret, err
		}
	}
	return ret, nil
}

// SyncConfig updates resource list from a file
func (r *Ranch) SyncConfig(configPath string) error {
	config, err := common.ParseConfig(configPath)
	if err != nil {
		return err
	}
	if err := common.ValidateConfig(config); err != nil {
		return err
	}
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()
	if err := r.Storage.SyncResources(config); err != nil {
		return err
	}
	return nil
}

// StartDynamicResourceUpdater starts a goroutine which periodically
// updates all dynamic resources.
func (r *Ranch) StartDynamicResourceUpdater(updatePeriod time.Duration) {
	if updatePeriod == 0 {
		return
	}
	go func() {
		updateTick := time.NewTicker(updatePeriod).C
		for {
			select {
			case <-updateTick:
				// TODO(ixdy): do we really need to acquire this lock?
				r.resourcesLock.Lock()
				r.Storage.UpdateAllDynamicResources()
				r.resourcesLock.Unlock()
			}
		}
	}()
}

// StartRequestGC starts the GC of expired requests
func (r *Ranch) StartRequestGC(gcPeriod time.Duration) {
	r.requestMgr.StartGC(gcPeriod)
}

// Metric returns a metric object with metrics filled in
func (r *Ranch) Metric(rtype string) (common.Metric, error) {
	metric := common.Metric{
		Type:    rtype,
		Current: map[string]int{},
		Owners:  map[string]int{},
	}

	resources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot find resources")
		return metric, &ResourceNotFound{rtype}
	}

	for _, res := range resources {
		if res.Type != rtype {
			continue
		}

		if _, ok := metric.Current[res.State]; !ok {
			metric.Current[res.State] = 0
		}

		if _, ok := metric.Owners[res.Owner]; !ok {
			metric.Owners[res.Owner] = 0
		}

		metric.Current[res.State]++
		metric.Owners[res.Owner]++
	}

	if len(metric.Current) == 0 && len(metric.Owners) == 0 {
		return metric, &ResourceNotFound{rtype}
	}

	return metric, nil
}
