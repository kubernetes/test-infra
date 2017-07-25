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
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
)

// Ranch is the place which all of the Resource objects lives.
type Ranch struct {
	Resources   []common.Resource
	lock        sync.RWMutex
	storagePath string
}

// Public errors:

// OwnerNotMatch will be returned if request owner does not match current owner for target resource.
type OwnerNotMatch struct {
	request string
	owner   string
}

func (o OwnerNotMatch) Error() string {
	return fmt.Sprintf("OwnerNotMatch - request by %s, currently owned by %s", o.request, o.owner)
}

// ResourceNotFound will be returned if requested resource does not exist.
type ResourceNotFound struct {
	name string
}

func (r ResourceNotFound) Error() string {
	return fmt.Sprintf("Resource %s not exist", r.name)
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
func NewRanch(config string, storage string) (*Ranch, error) {

	newRanch := &Ranch{
		storagePath: storage,
	}

	if storage != "" {
		buf, err := ioutil.ReadFile(storage)
		if err == nil {
			logrus.Infof("Current state: %v.", buf)
			err = json.Unmarshal(buf, newRanch)
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	if err := newRanch.SyncConfig(config); err != nil {
		return nil, err
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
func (r *Ranch) Acquire(rtype string, state string, dest string, owner string) (*common.Resource, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	for idx := range r.Resources {
		res := r.Resources[idx]
		if rtype == res.Type && state == res.State && res.Owner == "" {
			res.LastUpdate = time.Now()
			res.Owner = owner
			res.State = dest

			copy(r.Resources[idx:], r.Resources[idx+1:])
			r.Resources[len(r.Resources)-1] = res
			return &res, nil
		}
	}

	return nil, &ResourceNotFound{rtype}
}

// Release unsets owner for target resource and move it to a new state.
// In: name - name of the target resource
//     dest - destination state of the resource
//     owner - owner of the resource
// Out: nil on success, or
//      OwnerNotMatch error if owner does not match current owner of the resource, or
//      ResourceNotFound error if target named resource does not exist.
func (r *Ranch) Release(name string, dest string, owner string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	for idx := range r.Resources {
		res := &r.Resources[idx]
		if name == res.Name {
			if owner != res.Owner {
				return &OwnerNotMatch{res.Owner, owner}
			}
			res.LastUpdate = time.Now()
			res.Owner = ""
			res.State = dest
			return nil
		}
	}

	return &ResourceNotFound{name}
}

// Update updates the timestamp of a target resource.
// In: name - name of the target resource
//     state - current state of the resource
//     owner - current owner of the resource
// Out: nil on success, or
//      OwnerNotMatch error if owner does not match current owner of the resource, or
//      ResourceNotFound error if target named resource does not exist, or
//      StateNotMatch error if state does not match current state of the resource.
func (r *Ranch) Update(name string, owner string, state string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	for idx := range r.Resources {
		res := &r.Resources[idx]
		if name == res.Name {
			if owner != res.Owner {
				return &OwnerNotMatch{res.Owner, owner}
			}

			if state != res.State {
				return &StateNotMatch{res.State, state}
			}
			res.LastUpdate = time.Now()
			return nil
		}
	}

	return &ResourceNotFound{name}
}

// Reset unstucks a type of stale resource to a new state.
// In: rtype - type of the resource
//     state - current state of the resource
//     expire - duration before resource's last update
//     dest - destination state of expired resources
// Out: map of resource name - resource owner.
func (r *Ranch) Reset(rtype string, state string, expire time.Duration, dest string) map[string]string {
	r.lock.Lock()
	defer r.lock.Unlock()

	ret := make(map[string]string)

	for idx := range r.Resources {
		res := &r.Resources[idx]
		if rtype == res.Type && state == res.State && res.Owner != "" {
			if time.Now().Sub(res.LastUpdate) > expire {
				res.LastUpdate = time.Now()
				ret[res.Name] = res.Owner
				res.Owner = ""
				res.State = dest
			}
		}
	}

	return ret
}

// LogStatus outputs current status of all resources
func (r *Ranch) LogStatus() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	resJSON, err := json.Marshal(r.Resources)
	if err != nil {
		logrus.WithError(err).Errorf("Fail to marshal Resources. %v", r.Resources)
	}
	logrus.Infof("Current Resources : %v", string(resJSON))
}

// ResourceEntry is resource config format defined from resources.json
type ResourceEntry struct {
	Type  string   `json:"type"`
	State string   `json:"state"`
	Names []string `json:"names"`
}

// SyncConfig updates resource list from a file
func (r *Ranch) SyncConfig(config string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	data, err := r.ParseConfig(config)
	if err != nil {
		return err
	}

	r.syncConfigHelper(data)
	return nil
}

// ParseConfig reads in configPath and returns a list of resource objects
// on success.
func (r *Ranch) ParseConfig(configPath string) ([]common.Resource, error) {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	data := []ResourceEntry{}
	err = json.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}

	var resources []common.Resource
	for _, res := range data {
		for _, name := range res.Names {
			resources = append(resources, common.Resource{
				Type:  res.Type,
				State: res.State,
				Name:  name,
			})
		}
	}
	return resources, nil
}

// Boskos resource config will be updated every 10 mins.
// It will append newly added resources to ranch.Resources,
// And try to remove newly deleted resources from ranch.Resources.
// If the newly deleted resource is currently held by a user, the deletion will
// yield to next update cycle.
func (r *Ranch) syncConfigHelper(data []common.Resource) {
	// delete non-exist resource
	valid := 0
	for _, res := range r.Resources {
		// If currently busy, yield deletion to later cycles.
		if res.Owner != "" {
			r.Resources[valid] = res
			valid++
			continue
		}

		for _, newRes := range data {
			if res.Name == newRes.Name {
				r.Resources[valid] = res
				valid++
				break
			}
		}
	}
	r.Resources = r.Resources[:valid]

	// add new resource
	for _, p := range data {
		found := false
		for _, exist := range r.Resources {
			if p.Name == exist.Name {
				found = true
				break
			}
		}

		if !found {
			if p.State == "" {
				p.State = "free"
			}
			r.Resources = append(r.Resources, p)
		}
	}
}

// SaveState saves current server state in json format
func (r *Ranch) SaveState() {
	if r.storagePath == "" {
		return
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	// If fail to save data, fatal and restart the server
	if buf, err := json.Marshal(r); err != nil {
		logrus.WithError(err).Fatal("Error marshal ranch")
	} else if err = ioutil.WriteFile(r.storagePath+".tmp", buf, 0644); err != nil {
		logrus.WithError(err).Fatal("Error write file")
	} else if err = os.Rename(r.storagePath+".tmp", r.storagePath); err != nil {
		logrus.WithError(err).Fatal("Error rename file")
	}
}

// Metric returns a metric object with metrics filled in
func (r *Ranch) Metric(rtype string) (common.Metric, error) {
	metric := common.Metric{
		Type:    rtype,
		Current: map[string]int{},
		Owners:  map[string]int{},
	}

	for _, res := range r.Resources {
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
