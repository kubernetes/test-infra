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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/boskos/common"
)

// Ranch is the place which all of the Resource objects lives.
type Ranch struct {
	Storage       *Storage
	resourcesLock sync.RWMutex
	// For testing
	UpdateTime func() time.Time
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
	return fmt.Sprintf("Resource %s not exist", r.name)
}

// OwnerNotMatch will be returned if request owner does not match current owner for target resource.
type OwnerNotMatch struct {
	request string
	owner   string
}

func (o OwnerNotMatch) Error() string {
	return fmt.Sprintf("OwnerNotMatch - request by %s, currently owned by %s", o.request, o.owner)
}

// StateNotMatch will be returned if requested state does not match current state for target resource.
type StateNotMatch struct {
	expect  string
	current string
}

func (s StateNotMatch) Error() string {
	return fmt.Sprintf("StateNotMatch - expect %v, current %v", s.expect, s.current)
}

// NewRanch creates a new Ranch object.
// In: config - path to resource file
//     storage - path to where to save/restore the state data
// Out: A Ranch object, loaded from config/storage, or error
func NewRanch(config string, s *Storage) (*Ranch, error) {
	newRanch := &Ranch{
		Storage:    s,
		UpdateTime: updateTime,
	}
	if config != "" {
		if err := newRanch.SyncConfig(config); err != nil {
			return nil, err
		}
	}
	newRanch.LogStatus()
	return newRanch, nil
}

// Acquire checks out a type of resource in certain state without an owner,
// and move the checked out resource to the end of the resource list.
// In: rtype - name of the target resource
//     state - current state of the requested resource
//     dest - destination state of the requested resource
//     owner - requester of the resource
// Out: A valid Resource object on success, or
//      ResourceNotFound error if target type resource does not exist in target state.
func (r *Ranch) Acquire(rType, state, dest, owner string) (*common.Resource, error) {
	r.resourcesLock.Lock()
	defer r.resourcesLock.Unlock()

	resources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Errorf("could not get resources")
		return nil, &ResourceNotFound{rType}
	}

	for idx := range resources {
		res := resources[idx]
		if rType == res.Type && state == res.State && res.Owner == "" {
			res.LastUpdate = r.UpdateTime()
			res.Owner = owner
			res.State = dest
			if err := r.Storage.UpdateResource(res); err != nil {
				logrus.WithError(err).Errorf("could not update resource %s", res.Name)
				return nil, err
			}
			return &res, nil
		}
	}
	return nil, &ResourceNotFound{rType}
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

	rNames := map[string]bool{}
	for _, t := range names {
		rNames[t] = true
	}

	allResources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Errorf("could not get resources")
		return nil, &ResourceNotFound{state}
	}

	var resources []common.Resource

	for idx := range allResources {
		res := allResources[idx]
		if state == res.State {
			if res.Owner != "" {
				continue
			}
			if rNames[res.Name] {
				res.LastUpdate = r.UpdateTime()
				res.Owner = owner
				res.State = dest
				if err := r.Storage.UpdateResource(res); err != nil {
					logrus.WithError(err).Errorf("could not update resource %s", res.Name)
					return nil, err
				}
				resources = append(resources, res)
				delete(rNames, res.Name)
			}
		}
	}
	if len(rNames) != 0 {
		var missingResources []string
		for n := range rNames {
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
		return &OwnerNotMatch{res.Owner, owner}
	}
	res.LastUpdate = r.UpdateTime()
	res.Owner = ""
	res.State = dest
	if err := r.Storage.UpdateResource(res); err != nil {
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
		return &OwnerNotMatch{owner, res.Owner}
	}
	if state != res.State {
		return &StateNotMatch{res.State, state}
	}
	if res.UserData == nil {
		res.UserData = &common.UserData{}
	}
	res.UserData.Update(ud)
	res.LastUpdate = r.UpdateTime()
	if err := r.Storage.UpdateResource(res); err != nil {
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
		if rtype == res.Type && state == res.State && res.Owner != "" {
			if time.Since(res.LastUpdate) > expire {
				res.LastUpdate = r.UpdateTime()
				ret[res.Name] = res.Owner
				res.Owner = ""
				res.State = dest
				if err := r.Storage.UpdateResource(res); err != nil {
					logrus.WithError(err).Errorf("could not update resource %s", res.Name)
					return ret, err
				}
			}
		}
	}
	return ret, nil
}

// LogStatus outputs current status of all resources
func (r *Ranch) LogStatus() {
	resources, err := r.Storage.GetResources()

	if err != nil {
		return
	}

	resJSON, err := json.Marshal(resources)
	if err != nil {
		logrus.WithError(err).Errorf("Fail to marshal Resources. %v", resources)
	}
	logrus.Infof("Current Resources : %v", string(resJSON))
}

// SyncConfig updates resource list from a file
func (r *Ranch) SyncConfig(config string) error {
	resources, err := ParseConfig(config)
	if err != nil {
		return err
	}
	if err := r.Storage.SyncResources(resources); err != nil {
		return err
	}
	return nil
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
