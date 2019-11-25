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

package ranch

import (
	"fmt"
	"testing"
	"time"
)

const (
	testTTL      = time.Millisecond
	testGCPeriod = 20 * time.Millisecond
)

func TestRequestQueue(t *testing.T) {
	now := time.Now()
	rq := newRequestQueue()
	count := 10
	for i := 0; i < count; i++ {
		rank, new := rq.getRank(fmt.Sprintf("request_%d", i), testTTL, now)
		if rank != i+1 {
			t.Errorf("expected %d got %d", i+1, rank)
		}
		if !new {
			t.Errorf("should be new")
		}
	}
	rank, new := rq.getRank("", testTTL, now)
	if rank != count+1 {
		t.Errorf("expected %d got %d", count+1, rank)
	}
	if new {
		t.Errorf("empty request id should not be considered as new")
	}
	for i := 0; i < count; i++ {
		rq.delete(fmt.Sprintf("request_%d", i))
		for j := i + 1; j < count; j++ {
			rank, new := rq.getRank(fmt.Sprintf("request_%d", j), testTTL, now)
			if rank != j-i {
				t.Errorf("expected %d got %d", j-i, rank)
			}
			if new {
				t.Errorf("request id already exist")
			}
		}
		if rank, _ := rq.getRank("", testTTL, now); rank != count-i {
			t.Errorf("expected %d got %d", count-i, rank)
		}
		rq.cleanup(now)
		// cleanup should not impact result
		for j := i + 1; j < count; j++ {
			rank, new := rq.getRank(fmt.Sprintf("request_%d", j), testTTL, now)
			if rank != j-i {
				t.Errorf("expected %d got %d", j-i, rank)
			}
			if new {
				t.Errorf("request id already exist")
			}
		}
		if rank, _ := rq.getRank("", testTTL, now); rank != count-i {
			t.Errorf("expected %d got %d", count-i, rank)
		}
	}
	if !rq.isEmpty() {
		t.Errorf("RQ should be empty")
	}

}

func TestRequestManager(t *testing.T) {
	key := "key"
	id := "request1234"
	expectedRank := 1
	expectedRankEmpty := 2
	expectedRankAfterDelete := 1
	now := time.Now()
	expiredFuture := now.Add(2 * testTTL)
	mgr := NewRequestManager(testTTL)
	mgr.now = func() time.Time { return now }

	// Getting Rank
	rank, _ := mgr.GetRank(key, id)
	emptyRank, _ := mgr.GetRank(key, "")
	if rank != expectedRank {
		t.Errorf("expected rank %d got %d", expectedRank, rank)
	}
	if emptyRank != expectedRankEmpty {
		t.Errorf("expected empty rank %d got %d", expectedRankEmpty, emptyRank)
	}

	// Deleting
	mgr.Delete(key, id)
	afterDeleteRank, _ := mgr.GetRank(key, "")
	if afterDeleteRank != expectedRankAfterDelete {
		t.Errorf("expected empty rank %d got %d", expectedRankAfterDelete, afterDeleteRank)
	}

	// Re-adding request
	mgr.GetRank(key, id)

	// Starting cleanup
	mgr.cleanup(expiredFuture)
	afterDeleteRank, _ = mgr.GetRank(key, "")
	if afterDeleteRank != expectedRankAfterDelete {
		t.Errorf("expected empty rank %d got %d", expectedRankAfterDelete, afterDeleteRank)
	}
}

func TestRequestManager_GC(t *testing.T) {
	key := "key"
	id := "request1234"
	mgr := NewRequestManager(testTTL)

	// Starting GC
	mgr.StartGC(testGCPeriod)

	// Getting Rank
	rank, _ := mgr.GetRank(key, id)
	if rank != 1 {
		t.Errorf("expected rank %d got %d", 1, rank)
	}

	// Waiting for GC to happen
	time.Sleep(2 * testGCPeriod)

	// Checking
	rank, _ = mgr.GetRank(key, "")
	if rank != 1 {
		t.Errorf("expected empty rank %d got %d", 1, rank)
	}

	done := make(chan bool)
	go func() {
		mgr.StopGC()
		done <- true
	}()
	select {
	case <-done:
	// OK
	case <-time.After(2 * testGCPeriod):
		t.Errorf("could not STOP GC")
	}
}
