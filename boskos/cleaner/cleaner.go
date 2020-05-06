/*
Copyright 2019 The Kubernetes Authors.

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

package cleaner

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/mason"
)

type boskosClient interface {
	Acquire(rtype, state, dest string) (*common.Resource, error)
	AcquireByState(state, dest string, names []string) ([]common.Resource, error)
	ReleaseOne(name, dest string) error
	UpdateOne(name, state string, userData *common.UserData) error
	ReleaseAll(dest string) error
}

// Cleaner looks for ToBeDeleted resources and mark them as Tombstone.
// It also releases resource hold by dynamic resources if any.
type Cleaner struct {
	client boskosClient
	// TODO(sebastienvas): Make this dynamic
	cleanerCount     int
	storage          cleanerStorage
	boskosWaitPeriod time.Duration
	wg               sync.WaitGroup
	cancel           context.CancelFunc
}

type cleanerStorage interface {
	GetDynamicResourceLifeCycles() (*crds.DRLCObjectList, error)
}

// NewCleaner creates and initialized a new Cleaner object
// In: cleanerCount      - Number of cleaning threads
//     client            - boskos client
//     waitPeriod        - time to wait before a new boskos operation (acquire mostly)
// Out: A Pointer to a Cleaner Object
func NewCleaner(cleanerCount int, client boskosClient, waitPeriod time.Duration, s cleanerStorage) *Cleaner {
	return &Cleaner{
		client:           client,
		cleanerCount:     cleanerCount,
		storage:          s,
		boskosWaitPeriod: waitPeriod,
	}
}

func (c *Cleaner) recycleAll(ctx context.Context) {
	defer func() {
		logrus.Info("Exiting recycleAll Thread")
		c.wg.Done()
	}()
	tick := time.NewTicker(c.boskosWaitPeriod).C
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			dRLCs, err := c.storage.GetDynamicResourceLifeCycles()
			if err != nil {
				logrus.WithError(err).Warn("could not get resources")
				continue
			}
			for _, r := range dRLCs.Items {
				if res, err := c.client.Acquire(r.Name, common.ToBeDeleted, common.Cleaning); err != nil {
					logrus.WithError(err).Debug("boskos acquire failed!")
				} else {
					c.recycleOne(res)
					if err := c.client.ReleaseOne(res.Name, common.Tombstone); err != nil {
						logrus.WithError(err).Errorf("failed to release %s as %s", res.Name, common.Tombstone)
					} else {
						logrus.Infof("Released %s as %s", res.Name, common.Tombstone)
					}
				}
			}
		}
	}
}

func (c *Cleaner) recycleOne(res *common.Resource) {
	RecycleOne(c.client, res)
}

type RecycleBoskosClient interface {
	AcquireByState(state, dest string, names []string) ([]common.Resource, error)
	ReleaseOne(name, dest string) error
	UpdateOne(name, state string, userData *common.UserData) error
}

func RecycleOne(client RecycleBoskosClient, res *common.Resource) {
	logrus.Infof("Resource %s is being recycled", res.Name)
	leasedResources, err := mason.CheckUserData(*res)
	if err != nil {
		logrus.Warningf("could not find leased resources for %s", res.Name)
		return
	}
	if leasedResources != nil {
		resources, err := client.AcquireByState(res.Name, common.Cleaning, leasedResources)
		if err != nil {
			logrus.WithError(err).Warningf("could not acquire some leased resources for %s", res.Name)
		}
		for _, r := range resources {
			if err := client.ReleaseOne(r.Name, common.Dirty); err != nil {
				logrus.WithError(err).Warningf("could not release resource %s", r.Name)
			} else {
				logrus.Infof("resource %s released as %s", r.Name, common.Dirty)
			}
		}
		// Deleting Leased Resources
		res.UserData.Delete(mason.LeasedResources)
		if err := client.UpdateOne(res.Name, res.State, common.UserDataFromMap(map[string]string{mason.LeasedResources: ""})); err != nil {
			logrus.WithError(err).Errorf("could not update resource %s with freed leased resources", res.Name)
		}
	}
}

func (c *Cleaner) start(ctx context.Context, fn func(context.Context), count int) {
	for i := 0; i < count; i++ {
		go func() {
			fn(ctx)
		}()
		c.wg.Add(1)
	}
}

// Start Cleaner
func (c *Cleaner) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.start(ctx, c.recycleAll, c.cleanerCount)
	logrus.Info("Cleaner started")
}

// Stop Cleaner
func (c *Cleaner) Stop() {
	logrus.Info("Stopping Cleaner")
	c.cancel()
	c.wg.Wait()
	c.client.ReleaseAll(common.Dirty)
	logrus.Info("Cleaner stopped")
}
