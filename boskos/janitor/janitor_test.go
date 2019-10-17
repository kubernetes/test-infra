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

package main

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/test-infra/boskos/common"
)

type fakeBoskos struct {
	lock      sync.Mutex
	wg        sync.WaitGroup
	resources []common.Resource
}

// Create a fake client
func createFakeBoskos(resources int, types []string) *fakeBoskos {
	fb := &fakeBoskos{}
	r := rand.New(rand.NewSource(99))
	for i := 0; i < resources; i++ {
		fb.resources = append(fb.resources,
			common.Resource{
				Name:  fmt.Sprintf("res-%d", i),
				Type:  types[r.Intn(len(types))],
				State: common.Dirty,
			})
	}

	return fb
}

func (fb *fakeBoskos) Acquire(rtype string, state string, dest string) (*common.Resource, error) {
	fb.lock.Lock()
	defer fb.lock.Unlock()

	for idx := range fb.resources {
		r := &fb.resources[idx]
		if r.State == state {
			r.State = dest
			fb.wg.Add(1)
			return r, nil
		}
	}

	return nil, fmt.Errorf("could not find resource of type %s", rtype)
}

func (fb *fakeBoskos) ReleaseOne(name string, dest string) error {
	fb.lock.Lock()
	defer fb.lock.Unlock()

	for idx := range fb.resources {
		r := &fb.resources[idx]
		if r.Name == name {
			r.State = dest
			fb.wg.Done()
			return nil
		}
	}

	return fmt.Errorf("no resource %v", name)
}

func (fb *fakeBoskos) SyncAll() error {
	return nil
}

// waitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

func TestNormal(t *testing.T) {
	var totalClean int32

	fakeClean := func(resource *common.Resource, extraFlags []string) error {
		atomic.AddInt32(&totalClean, 1)
		return nil
	}

	types := []string{"a", "b", "c", "d"}
	fb := createFakeBoskos(1000, types)

	buffer := setup(fb, poolSize, bufferSize, fakeClean, nil)
	totalAcquire := run(fb, buffer, []string{"t"})

	if totalAcquire != len(fb.resources) {
		t.Errorf("expect to acquire all resources(%d) from fake boskos, got %d", len(fb.resources), totalAcquire)
	}

	if waitTimeout(&fb.wg, time.Second) {
		t.Fatal("expect janitor to finish!")
	}

	if int(totalClean) != len(fb.resources) {
		t.Errorf("expect to clean all resources(%d) from fake boskos, got %d", len(fb.resources), totalClean)
	}

	for _, r := range fb.resources {
		if r.State != common.Free {
			t.Errorf("resource %v, expect state free, got state %v", r.Name, r.State)
		}
	}
}

func FakeRun(fb *fakeBoskos, buffer chan<- *common.Resource, res string) (int, error) {
	timeout := time.NewTimer(5 * time.Second).C

	totalClean := 0
	maxAcquire := poolSize + bufferSize + 1

	for {
		select {
		case <-timeout:
			return totalClean, errors.New("should not timedout")
		default:
			if resource, err := fb.Acquire(res, common.Dirty, common.Cleaning); err != nil {
				return totalClean, fmt.Errorf("acquire failed with %v", err)
			} else if resource.Name == "" {
				return totalClean, errors.New("not expect to run out of resources")
			} else {
				if totalClean > maxAcquire {
					// poolSize in janitor, bufferSize more in janitor pool, 1 more hanging and will exit the loop
					return totalClean, fmt.Errorf("should not acquire more than %d projects", maxAcquire)
				}
				boom := time.After(50 * time.Millisecond)
				select {
				case buffer <- resource: // normal case
					totalClean++
				case <-boom:
					return totalClean, nil
				}
			}
		}
	}
}

func TestMalfunctionJanitor(t *testing.T) {

	stuck := make(chan string, 1)
	fakeClean := func(resource *common.Resource, extraFlags []string) error {
		<-stuck
		return nil
	}

	fb := createFakeBoskos(200, []string{"t"})

	buffer := setup(fb, poolSize, bufferSize, fakeClean, nil)

	if totalClean, err := FakeRun(fb, buffer, "t"); err != nil {
		t.Fatalf("run failed unexpectedly : %v", err)
	} else if totalClean != poolSize+1 {
		t.Errorf("expect to clean %d from fake boskos, got %d", poolSize+1, totalClean)
	}
}
