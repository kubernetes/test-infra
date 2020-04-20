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
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
)

// Ranch is the place which all of the Resource objects lives.
type Ranch struct {
	Storage    *Storage
	requestMgr *RequestManager
	//
	now func() time.Time
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
	return fmt.Sprintf("resource type %q does not exist", r.rType)
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
func (r *Ranch) Acquire(rType, state, dest, owner, requestID string) (*crds.ResourceObject, error) {
	logger := logrus.WithFields(logrus.Fields{
		"type":       rType,
		"state":      state,
		"dest":       dest,
		"owner":      owner,
		"identifier": requestID,
	})

	var returnRes *crds.ResourceObject
	if err := retryOnConflict(retry.DefaultBackoff, func() error {
		logger.Debug("Determining request priority...")
		ts := acquireRequestPriorityKey{rType: rType, state: state}
		rank, new := r.requestMgr.GetRank(ts, requestID)
		logger.WithFields(logrus.Fields{"rank": rank, "new": new}).Debug("Determined request priority.")

		resources, err := r.Storage.GetResources()
		if err != nil {
			logger.WithError(err).Errorf("could not get resources")
			return &ResourceNotFound{rType}
		}
		logger.Debugf("Considering %d resources.", len(resources.Items))

		// For request priority we need to go over all the list until a matching rank
		matchingResoucesCount := 0
		typeCount := 0
		for idx := range resources.Items {
			res := resources.Items[idx]
			if rType != res.Spec.Type {
				continue
			}
			typeCount++

			if state != res.Status.State || res.Status.Owner != "" {
				continue
			}
			matchingResoucesCount++

			if matchingResoucesCount < rank {
				continue
			}
			logger = logger.WithField("resource", res.Name)
			res.Status.Owner = owner
			res.Status.State = dest
			logger.Debug("Updating resource.")
			updatedRes, err := r.Storage.UpdateResource(&res)
			if err != nil {
				return err
			}
			// Deleting this request since it has been fulfilled
			if requestID != "" {
				logger.Debug("Cleaning up requests.")
				r.requestMgr.Delete(ts, requestID)
			}
			logger.Debug("Successfully acquired resource.")
			returnRes = updatedRes
			return nil
		}

		if new {
			logger.Debug("Checking for associated dynamic resource type...")
			lifeCycle, err := r.Storage.GetDynamicResourceLifeCycle(rType)
			// Assuming error means no associated dynamic resource.
			if err == nil {
				if typeCount < lifeCycle.Spec.MaxCount {
					logger.Debug("Adding new dynamic resources...")
					res := newResourceFromNewDynamicResourceLifeCycle(r.Storage.generateName(), lifeCycle, r.now())
					if err := r.Storage.AddResource(res); err != nil {
						logger.WithError(err).Warningf("unable to add a new resource of type %s", rType)
					}
					logger.Infof("Added dynamic resource %s of type %s", res.Name, res.Spec.Type)
				}
			} else {
				logrus.WithError(err).Debug("Failed listing DRLC")
			}
		}

		if typeCount > 0 {
			return &ResourceNotFound{rType}
		}
		return &ResourceTypeNotFound{rType}
	}); err != nil {
		switch err.(type) {
		case *ResourceNotFound:
			// This error occurs when there are no more resources to lease out.
			// Such a condition is a normal and expected part of operation, so
			// it does not warrant an error log.
		default:
			logrus.WithError(err).Error("Acquire failed")
		}
		return nil, err
	}

	return returnRes, nil
}

// AcquireByState checks out resources of a given type without an owner,
// that matches a list of resources names.
// In: state - current state of the requested resource
//     dest - destination state of the requested resource
//     owner - requester of the resource
//     names - names of resource to acquire
// Out: A valid list of Resource object on success, or
//      ResourceNotFound error if target type resource does not exist in target state.
func (r *Ranch) AcquireByState(state, dest, owner string, names []string) ([]*crds.ResourceObject, error) {
	if names == nil {
		return nil, fmt.Errorf("must provide names of expected resources")
	}

	var returnRes []*crds.ResourceObject
	if err := retryOnConflict(retry.DefaultBackoff, func() error {
		rNames := sets.NewString(names...)

		allResources, err := r.Storage.GetResources()
		if err != nil {
			logrus.WithError(err).Errorf("could not get resources")
			return &ResourceNotFound{state}
		}

		var resources []*crds.ResourceObject

		for idx := range allResources.Items {
			res := allResources.Items[idx]
			if state != res.Status.State || res.Status.Owner != "" || !rNames.Has(res.Name) {
				continue
			}

			res.Status.Owner = owner
			res.Status.State = dest
			updatedRes, err := r.Storage.UpdateResource(&res)
			if err != nil {
				return err
			}
			resources = append(resources, updatedRes)
			rNames.Delete(res.Name)
		}

		if rNames.Len() != 0 {
			missingResources := rNames.List()
			err := &ResourceNotFound{state}
			logrus.WithError(err).Errorf("could not find required resources %s", strings.Join(missingResources, ", "))
			returnRes = resources
			return err
		}
		returnRes = resources
		return nil
	}); err != nil {
		logrus.WithError(err).Error("AcquireByState failed")
		// Not a bug, we return what we got even on error.
		return returnRes, err
	}

	return returnRes, nil
}

// Release unsets owner for target resource and move it to a new state.
// In: name - name of the target resource
//     dest - destination state of the resource
//     owner - owner of the resource
// Out: nil on success, or
//      OwnerNotMatch error if owner does not match current owner of the resource, or
//      ResourceNotFound error if target named resource does not exist.
func (r *Ranch) Release(name, dest, owner string) error {
	if err := retryOnConflict(retry.DefaultBackoff, func() error {
		res, err := r.Storage.GetResource(name)
		if err != nil {
			logrus.WithError(err).Errorf("unable to release resource %s", name)
			return &ResourceNotFound{name}
		}
		if owner != res.Status.Owner {
			return &OwnerNotMatch{request: owner, owner: res.Status.Owner}
		}

		res.Status.Owner = ""
		res.Status.State = dest

		if lf, err := r.Storage.GetDynamicResourceLifeCycle(res.Spec.Type); err == nil {
			// Assuming error means not existing as the only way to differentiate would be to list
			// all resources and find the right one which is more costly.
			if lf.Spec.LifeSpan != nil {
				expirationTime := r.now().Add(*lf.Spec.LifeSpan)
				res.Status.ExpirationDate = &expirationTime
			}
		} else {
			res.Status.ExpirationDate = nil
		}

		if _, err := r.Storage.UpdateResource(res); err != nil {
			return err
		}
		return nil
	}); err != nil {
		logrus.WithError(err).Error("Release failed")
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
	if err := retryOnConflict(retry.DefaultBackoff, func() error {
		res, err := r.Storage.GetResource(name)
		if err != nil {
			logrus.WithError(err).Errorf("could not find resource %s for update", name)
			return &ResourceNotFound{name}
		}
		if owner != res.Status.Owner {
			return &OwnerNotMatch{request: owner, owner: res.Status.Owner}
		}
		if state != res.Status.State {
			return &StateNotMatch{res.Status.State, state}
		}
		if res.Status.UserData == nil {
			res.Status.UserData = &common.UserData{}
		}
		res.Status.UserData.Update(ud)
		if _, err := r.Storage.UpdateResource(res); err != nil {
			return err
		}
		return nil
	}); err != nil {
		logrus.WithError(err).Error("Update failed")
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
	var ret map[string]string
	if err := retryOnConflict(retry.DefaultBackoff, func() error {
		ret = make(map[string]string)
		resources, err := r.Storage.GetResources()
		if err != nil {
			return err
		}

		for idx := range resources.Items {
			res := resources.Items[idx]
			if rtype != res.Spec.Type || state != res.Status.State || res.Status.Owner == "" || r.now().Sub(res.Status.LastUpdate) < expire {
				continue
			}

			ret[res.Name] = res.Status.Owner
			res.Status.Owner = ""
			res.Status.State = dest
			if _, err := r.Storage.UpdateResource(&res); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		logrus.WithError(err).Error("Reset failed")
		return nil, err
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
	return r.Storage.SyncResources(config)
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
				if err := r.Storage.UpdateAllDynamicResources(); err != nil {
					logrus.WithError(err).Error("UpdateAllDynamicResources failed")
				}
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
	metric := common.NewMetric(rtype)

	resources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot find resources")
		return metric, &ResourceNotFound{rtype}
	}

	for _, res := range resources.Items {
		if res.Spec.Type != rtype {
			continue
		}

		metric.Current[res.Status.State]++
		metric.Owners[res.Status.Owner]++
	}

	if len(metric.Current) == 0 && len(metric.Owners) == 0 {
		return metric, &ResourceNotFound{rtype}
	}

	return metric, nil
}

// AllMetrics returns a list of Metric objects for all resource types.
func (r *Ranch) AllMetrics() ([]common.Metric, error) {
	resources, err := r.Storage.GetResources()
	if err != nil {
		logrus.WithError(err).Error("cannot get resources")
		return nil, err
	}

	metrics := map[string]common.Metric{}

	for _, res := range resources.Items {
		metric, ok := metrics[res.Spec.Type]
		if !ok {
			metric = common.NewMetric(res.Spec.Type)
			metrics[res.Spec.Type] = metric
		}

		metric.Current[res.Status.State]++
		metric.Owners[res.Status.Owner]++
	}

	result := make([]common.Metric, 0, len(metrics))
	for _, metric := range metrics {
		result = append(result, metric)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Type < result[j].Type
	})
	return result, nil
}

// newResourceFromNewDynamicResourceLifeCycle creates a resource from DynamicResourceLifeCycle given a name and a time.
// Using this method helps make sure all the resources are created the same way.
func newResourceFromNewDynamicResourceLifeCycle(name string, dlrc *crds.DRLCObject, now time.Time) *crds.ResourceObject {
	return crds.NewResource(name, dlrc.Name, dlrc.Spec.InitialState, "", now)
}

func retryOnConflict(backoff wait.Backoff, fn func() error) error {
	return retry.OnError(backoff, isConflict, fn)
}

func isConflict(err error) bool {
	if kerrors.IsConflict(err) {
		return true
	}
	if x, ok := err.(interface{ Unwrap() error }); ok {
		return isConflict(x.Unwrap())
	}
	if aggregate, ok := err.(utilerrors.Aggregate); ok {
		for _, err := range aggregate.Errors() {
			if isConflict(err) {
				return true
			}
		}
	}
	return false
}
