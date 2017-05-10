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
	"time"

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
			fmt.Print("fb.wg.Add\n")
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
			fmt.Print("fb.wg.Done\n")
			fb.wg.Done()
			return nil
		}
	}

	return fmt.Errorf("No resource %v", name)
}

func TestDrainAll(t *testing.T) {
	oldClean := clean
	defer func() { clean = oldClean }()
	clean = func(p string) error {
		time.Sleep(3 * time.Second)
		return nil
	}

	sem := make(semaphore, 3)
	errors := make(chan error, 0)

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
		},
	}
	if err := update(fb, sem, errors); err != nil {
		t.Fatalf("Unexpect error from first update: %v", err)
	}

	// Make sure all clean route finishes
	fb.wg.Wait()

	// check error from previous executions
	if err := update(fb, sem, errors); err != nil {
		t.Fatalf("Unexpect error from second update: %v", err)
	}

	for _, r := range fb.resources {
		if r.State != "free" {
			t.Errorf("Resource %v, expect state free, got state %v", r.Name, r.State)
		}
	}
}

func TestDrainTwice(t *testing.T) {
	oldClean := clean
	defer func() { clean = oldClean }()
	clean = func(p string) error {
		time.Sleep(time.Second)
		return nil
	}

	sem := make(semaphore, 2)
	errors := make(chan error, 1)

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
		},
	}
	if err := update(fb, sem, errors); err == nil {
		t.Fatal("Expect error, got nil")
	} else if err.Error() != "channel is full" {
		t.Fatalf("Unexpect error from first update: %v", err)
	}

	// Make sure first two clean route finishes
	fb.wg.Wait()

	// check error from previous executions
	if err := update(fb, sem, errors); err != nil {
		t.Fatalf("Unexpect error from second update: %v", err)
	}

	// Make sure the third clean route finishes
	fb.wg.Wait()

	// check error from previous executions
	if err := update(fb, sem, errors); err != nil {
		t.Fatalf("Unexpect error from third update: %v", err)
	}

	for _, r := range fb.resources {
		if r.State != "free" {
			t.Errorf("Resource %v, expect state free, got state %v", r.Name, r.State)
		}
	}
}
