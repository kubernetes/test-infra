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
func CreateFakeBoskos(resources int) *fakeBoskos {
	fb := &fakeBoskos{}
	for i := 0; i < resources; i++ {
		fb.resources = append(fb.resources,
			common.Resource{
				Name:  fmt.Sprintf("res-%d", i),
				Type:  "project",
				State: "dirty",
			})
	}

	return fb
}

func (fb *fakeBoskos) Acquire(rtype string, state string, dest string) (string, error) {
	fb.lock.Lock()
	defer fb.lock.Unlock()

	for idx := range fb.resources {
		r := &fb.resources[idx]
		if r.State == state {
			r.State = dest
			fb.wg.Add(1)
			return r.Name, nil
		}
	}

	return "", nil
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

	return fmt.Errorf("No resource %v", name)
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
	var totalClean int32 = 0

	oldClean := clean
	defer func() { clean = oldClean }()
	clean = func(p string) error {
		atomic.AddInt32(&totalClean, 1)
		return nil
	}

	fb := CreateFakeBoskos(1000)

	buffer := setup(fb, poolSize, bufferSize)
	totalAcquire := run(fb, buffer)

	if totalAcquire != len(fb.resources) {
		t.Errorf("Expect to acquire all resources(%d) from fake boskos, got %d", len(fb.resources), totalAcquire)
	}

	if waitTimeout(&fb.wg, time.Second) {
		t.Fatal("Expect janitor to finish!")
	}

	if int(totalClean) != len(fb.resources) {
		t.Errorf("Expect to clean all resources(%d) from fake boskos, got %d", len(fb.resources), totalClean)
	}

	for _, r := range fb.resources {
		if r.State != "free" {
			t.Errorf("Resource %v, expect state free, got state %v", r.Name, r.State)
		}
	}
}

func FakeRun(fb *fakeBoskos, buffer chan string) (int, error) {
	timeout := time.NewTimer(5 * time.Second).C

	totalClean := 0

	for {
		select {
		case <-timeout:
			return totalClean, nil
		default:
			if proj, err := fb.Acquire("project", "dirty", "cleaning"); err != nil {
				return totalClean, fmt.Errorf("Acquire failed with %v", err)
			} else if proj == "" {
				return totalClean, errors.New("Not expect to run out of resources!")
			} else {
				if totalClean > 20 {
					// 10 in janitor, 11th in janitor pool, 12th hanging and will exit the loop
					return totalClean, errors.New("Should not acquire more than 12 projects!")
				}
				boom := time.After(50 * time.Millisecond)
				select {
				case buffer <- proj: // normal case
					totalClean++
				case <-boom:
					return totalClean, nil
				}
			}
		}
	}
}

func TestMalfunctionJanitor(t *testing.T) {

	oldClean := clean
	defer func() { clean = oldClean }()
	clean = func(p string) error {
		time.Sleep(time.Hour)
		return nil
	}

	fb := CreateFakeBoskos(100)

	buffer := setup(fb, poolSize, bufferSize)

	if totalClean, err := FakeRun(fb, buffer); err != nil {
		t.Fatalf("Run failed unexpectedly : %v", err)
	} else if totalClean != poolSize+1 {
		t.Errorf("Expect to clean %d from fake boskos, got %d", poolSize+1, totalClean)
	}
}
