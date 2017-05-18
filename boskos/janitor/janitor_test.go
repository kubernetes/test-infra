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
	"fmt"
	"sync"
	"testing"

	"k8s.io/test-infra/boskos/common"
)

type fakeBoskos struct {
	lock      sync.Mutex
	wg        sync.WaitGroup
	resources []common.Resource
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

func TestDrain(t *testing.T) {
	oldClean := clean
	defer func() { clean = oldClean }()
	clean = func(p string) error {
		return nil
	}

	janitorPool := make(chan string, 1)

	fb := &fakeBoskos{
		resources: []common.Resource{
			{
				Name:  "res-1",
				Type:  "project",
				State: "dirty",
			},
			{
				Name:  "res-2",
				Type:  "project",
				State: "dirty",
			},
			{
				Name:  "res-3",
				Type:  "project",
				State: "dirty",
			},
			{
				Name:  "res-4",
				Type:  "project",
				State: "dirty",
			},
			{
				Name:  "res-5",
				Type:  "project",
				State: "dirty",
			},
		},
	}

	for i := 0; i < 2; i++ {
		go janitor(fb, janitorPool)
	}

	var totalAcquire int

	for {
		if proj, err := fb.Acquire("project", "dirty", "cleaning"); err != nil {
			t.Fatalf("Acquire failed with %v", err)
		} else if proj == "" {
			break
		} else {
			totalAcquire++
			janitorPool <- proj // will block until pool has a free slot
		}
	}

	if totalAcquire != len(fb.resources) {
		t.Errorf("Expect to acquire all resources(5) from fake boskos, got %d", totalAcquire)
	}

	fb.wg.Wait()

	for _, r := range fb.resources {
		if r.State != "free" {
			t.Errorf("Resource %v, expect state free, got state %v", r.Name, r.State)
		}
	}
}
