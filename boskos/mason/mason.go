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

package mason

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
)

const (
	// LeasedResources is a common.UserData entry
	LeasedResources = "leasedResources"
)

// Masonable should be implemented by all configurations
type Masonable interface {
	Construct(context.Context, common.Resource, common.TypeToResources) (*common.UserData, error)
}

// ConfigConverter converts a string into a Masonable
type ConfigConverter func(string) (Masonable, error)

type storageAccess interface {
	// GetDynamicResourceLifeCycle gets an existing dynamic resource life cycle, errors otherwise
	GetDynamicResourceLifeCycle(name string) (common.DynamicResourceLifeCycle, error)

	// GetDynamicResourceLifeCycles list all dynamic resource life cycle
	GetDynamicResourceLifeCycles() ([]common.DynamicResourceLifeCycle, error)
}

type boskosClient interface {
	Acquire(rtype, state, dest string) (*common.Resource, error)
	AcquireByState(state, dest string, names []string) ([]common.Resource, error)
	ReleaseOne(name, dest string) error
	UpdateOne(name, state string, userData *common.UserData) error
	SyncAll() error
	UpdateAll(dest string) error
	ReleaseAll(dest string) error
}

// Mason uses config to convert dirty resources to usable one
type Mason struct {
	client                             boskosClient
	cleanerCount                       int
	storage                            storageAccess
	pending, fulfilled, cleaned        chan requirements
	boskosWaitPeriod, boskosSyncPeriod time.Duration
	wg                                 sync.WaitGroup
	configConverters                   map[string]ConfigConverter
	cancel                             context.CancelFunc
}

// requirements for a given resource
type requirements struct {
	resource    common.Resource
	needs       common.ResourceNeeds
	fulfillment common.TypeToResources
}

func (r requirements) isFulFilled() bool {
	for rType, count := range r.needs {
		resources, ok := r.fulfillment[rType]
		if !ok {
			return false
		}
		if len(resources) != count {
			return false
		}
	}
	return true
}

// NewMason creates and initialized a new Mason object
// In: rtypes            - A list of resource types to act on
//     cleanerCount      - Number of cleaning threads
//     client            - boskos client
//     waitPeriod        - time to wait before a new boskos operation (acquire mostly)
//     syncPeriod        - time to wait before syncing resource information to boskos
// Out: A Pointer to a Mason Object
func NewMason(cleanerCount int, client boskosClient, waitPeriod, syncPeriod time.Duration, s storageAccess) *Mason {
	return &Mason{
		client:           client,
		cleanerCount:     cleanerCount,
		storage:          s,
		pending:          make(chan requirements),
		cleaned:          make(chan requirements, cleanerCount+1),
		fulfilled:        make(chan requirements, cleanerCount+1),
		boskosWaitPeriod: waitPeriod,
		boskosSyncPeriod: syncPeriod,
		configConverters: map[string]ConfigConverter{},
	}
}

// CheckUserData helps with extracting leased resource data from the resource.
func CheckUserData(res common.Resource) (common.LeasedResources, error) {
	var leasedResources common.LeasedResources
	if res.UserData == nil {
		err := fmt.Errorf("user data is empty")
		return nil, err
	}

	if err := res.UserData.Extract(LeasedResources, &leasedResources); err != nil {
		return nil, err
	}
	return leasedResources, nil
}

// RegisterConfigConverter is used to register a new Masonable interface
// In: name - identifier for Masonable implementation
//     fn   - function that will parse the configuration string and return a Masonable interface
//
// Out: nil on success, error otherwise
func (m *Mason) RegisterConfigConverter(name string, fn ConfigConverter) error {
	_, ok := m.configConverters[name]
	if ok {
		return fmt.Errorf("a converter for %s already exists", name)
	}
	m.configConverters[name] = fn
	return nil
}

func (m *Mason) convertConfig(configEntry *common.DynamicResourceLifeCycle) (Masonable, error) {
	fn, ok := m.configConverters[configEntry.Config.Type]
	if !ok {
		return nil, fmt.Errorf("config type %s is not supported", configEntry.GetName())
	}
	return fn(configEntry.Config.Content)
}

func (m *Mason) garbageCollect(req requirements) {
	names := []string{req.resource.Name}

	for _, resources := range req.fulfillment {
		for _, r := range resources {
			names = append(names, r.Name)
		}
	}

	for _, name := range names {
		if err := m.client.ReleaseOne(name, common.Dirty); err != nil {
			logrus.WithError(err).Errorf("Unable to release leased resource %s", name)
		}
	}
}

func (m *Mason) cleanAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting cleanAll Thread")
		m.wg.Done()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.fulfilled:
			if err := m.cleanOne(ctx, &req.resource, req.fulfillment); err != nil {
				logrus.WithError(err).Errorf("unable to clean resource %s", req.resource.Name)
				m.garbageCollect(req)
			} else {
				m.cleaned <- req
			}
		}
	}
}

func (m *Mason) cleanOne(ctx context.Context, res *common.Resource, leasedResources common.TypeToResources) error {
	configEntry, err := m.storage.GetDynamicResourceLifeCycle(res.Type)
	if err != nil {
		logrus.WithError(err).Errorf("failed to get config for resource %s", res.Type)
		return err
	}
	config, err := m.convertConfig(&configEntry)
	if err != nil {
		logrus.WithError(err).Errorf("failed to convert config type %s - \n%s", configEntry.Config.Type, configEntry.Config.Content)
		return err
	}

	errChan := make(chan error)
	var userData *common.UserData

	go func() {
		var err error
		userData, err = config.Construct(ctx, *res, leasedResources.Copy())
		errChan <- err
	}()

	select {
	case err = <-errChan:
		if err != nil {
			logrus.WithError(err).Errorf("failed to construct resource %s", res.Name)
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := m.client.UpdateOne(res.Name, res.State, userData); err != nil {
		logrus.WithError(err).Error("unable to update user data")
		return err
	}
	if res.UserData == nil {
		res.UserData = userData
	} else {
		res.UserData.Update(userData)
	}
	logrus.Infof("Resource %s is cleaned", res.Name)
	return nil
}

func (m *Mason) freeAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting freeAll Thread")
		m.wg.Done()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.cleaned:
			if err := m.freeOne(&req.resource); err != nil {
				logrus.WithError(err).Errorf("failed to free up resource %s", req.resource.Name)
				m.garbageCollect(req)
			}
		}
	}
}

func (m *Mason) freeOne(res *common.Resource) error {
	leasedResources, err := CheckUserData(*res)
	if err != nil {
		return err
	}
	// TODO: Implement a ReleaseMultiple in a transaction to prevent orphans
	// Finally return the resource as free
	if err := m.client.ReleaseOne(res.Name, common.Free); err != nil {
		logrus.WithError(err).Errorf("failed to release resource %s", res.Name)
		return err
	}
	// And release leased resources as res.Name state
	for _, name := range leasedResources {
		if err := m.client.ReleaseOne(name, res.Name); err != nil {
			logrus.WithError(err).Errorf("unable to release %s to state %s", name, res.Name)
			return err
		}
	}
	logrus.Infof("Resource %s has been freed", res.Name)
	return nil
}

func (m *Mason) recycleAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting recycleAll Thread")
		m.wg.Done()
	}()
	tick := time.NewTicker(m.boskosWaitPeriod).C
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			configs, err := m.storage.GetDynamicResourceLifeCycles()
			if err != nil {
				logrus.WithError(err).Error("unable to get configuration")
				continue
			}
			var configTypes []string
			for _, c := range configs {
				configTypes = append(configTypes, c.GetName())
			}
			for _, r := range configTypes {
				if res, err := m.client.Acquire(r, common.Dirty, common.Cleaning); err != nil {
					logrus.WithError(err).Debug("boskos acquire failed!")
				} else {
					if req, err := m.recycleOne(res); err != nil {
						logrus.WithError(err).Errorf("unable to recycle resource %s", res.Name)
						if err := m.client.ReleaseOne(res.Name, common.Dirty); err != nil {
							logrus.WithError(err).Errorf("Unable to release resources %s", res.Name)
						}
					} else {
						m.pending <- *req
					}
				}
			}
		}
	}
}

func (m *Mason) recycleOne(res *common.Resource) (*requirements, error) {
	logrus.Infof("Resource %s is being recycled", res.Name)
	configEntry, err := m.storage.GetDynamicResourceLifeCycle(res.Type)
	if err != nil {
		logrus.WithError(err).Errorf("could not get config for resource type %s", res.Type)
		return nil, err
	}

	leasedResources, err := CheckUserData(*res)
	if err != nil {
		logrus.WithError(err).Warningf("failed to extract %s from resource user data", LeasedResources)
	}
	if leasedResources != nil {
		resources, err := m.client.AcquireByState(res.Name, common.Leased, leasedResources)
		if err != nil {
			logrus.WithError(err).Warningf("could not acquire any leased resources for %s", res.Name)
		}

		for _, r := range resources {
			if err := m.client.ReleaseOne(r.Name, common.Dirty); err != nil {
				logrus.WithError(err).Warningf("could not release resource %s", r.Name)
			}
		}
		// Deleting Leased Resources
		res.UserData.Delete(LeasedResources)
		if err := m.client.UpdateOne(res.Name, res.State, common.UserDataFromMap(map[string]string{LeasedResources: ""})); err != nil {
			logrus.WithError(err).Errorf("could not update resource %s with freed leased resources", res.Name)
		}
	}

	return &requirements{
		fulfillment: common.TypeToResources{},
		needs:       configEntry.Needs,
		resource:    *res,
	}, nil
}

func (m *Mason) syncAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting UpdateAll Thread")
		m.wg.Done()
	}()
	tick := time.NewTicker(m.boskosSyncPeriod).C
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			if err := m.client.SyncAll(); err != nil {
				logrus.WithError(err).Errorf("failed to sync resources")
			}
		}
	}
}

func (m *Mason) fulfillAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting fulfillAll Thread")
		m.wg.Done()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.pending:
			if err := m.fulfillOne(ctx, &req); err != nil {
				m.garbageCollect(req)
			} else {
				m.fulfilled <- req
			}
		}
	}
}

func (m *Mason) fulfillOne(ctx context.Context, req *requirements) error {
	// Making a copy
	needs := common.ResourceNeeds{}
	for k, v := range req.needs {
		needs[k] = v
	}
	tick := time.NewTicker(m.boskosWaitPeriod).C
	for rType := range needs {
		for needs[rType] > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick:
				m.updateResources(req)
				if res, err := m.client.Acquire(rType, common.Free, common.Leased); err != nil {
					logrus.WithError(err).Debug("boskos acquire failed!")
				} else {
					req.fulfillment[rType] = append(req.fulfillment[rType], *res)
					needs[rType]--
				}
			}
		}
	}
	if req.isFulFilled() {
		var leasedResources common.LeasedResources
		for _, lr := range req.fulfillment {
			for _, r := range lr {
				leasedResources = append(leasedResources, r.Name)
			}
		}
		userData := &common.UserData{}
		if err := userData.Set(LeasedResources, &leasedResources); err != nil {
			logrus.WithError(err).Errorf("failed to add %s user data", LeasedResources)
			return err
		}
		if err := m.client.UpdateOne(req.resource.Name, req.resource.State, userData); err != nil {
			logrus.WithError(err).Errorf("Unable to update resource %s", req.resource.Name)
			return err
		}
		if req.resource.UserData == nil {
			req.resource.UserData = userData
		} else {
			req.resource.UserData.Update(userData)
		}

		logrus.Infof("requirements for release %s is fulfilled", req.resource.Name)
		return nil
	}
	return nil
}

func (m *Mason) updateResources(req *requirements) {
	var resources []common.Resource
	resources = append(resources, req.resource)
	for _, leasedResources := range req.fulfillment {
		resources = append(resources, leasedResources...)
	}
	for _, r := range resources {
		if err := m.client.UpdateOne(r.Name, r.State, nil); err != nil {
			logrus.WithError(err).Warningf("failed to update resource %s", r.Name)
		}
	}
}

func (m *Mason) start(ctx context.Context, fn func(context.Context)) {
	go func() {
		fn(ctx)
	}()
	m.wg.Add(1)
}

// Start Mason
func (m *Mason) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.start(ctx, m.syncAll)
	m.start(ctx, m.recycleAll)
	m.start(ctx, m.fulfillAll)
	for i := 0; i < m.cleanerCount; i++ {
		m.start(ctx, m.cleanAll)
	}
	m.start(ctx, m.freeAll)
	logrus.Info("Mason started")
}

// Stop Mason
func (m *Mason) Stop() {
	logrus.Info("Stopping Mason")
	m.cancel()
	m.wg.Wait()
	close(m.pending)
	close(m.cleaned)
	close(m.fulfilled)
	m.client.ReleaseAll(common.Dirty)
	logrus.Info("Mason stopped")
}
