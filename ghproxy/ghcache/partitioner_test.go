/*
Copyright 2020 The Kubernetes Authors.

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

package ghcache

import (
	"net/http"
	"sync"
	"testing"
)

type fakeRoundTripperCreator struct {
	t             *testing.T
	lock          *sync.Mutex
	roundTrippers map[string]*fakeRoundTripper
}

func (frtc *fakeRoundTripperCreator) createRoundTripper(partitionKey string) http.RoundTripper {
	frtc.lock.Lock()
	defer frtc.lock.Unlock()
	_, alreadyExists := frtc.roundTrippers[partitionKey]
	if alreadyExists {
		frtc.t.Fatalf("creation of already existing partition %q was requested", partitionKey)
	}
	frtc.roundTrippers[partitionKey] = &fakeRoundTripper{lock: &sync.Mutex{}}
	return frtc.roundTrippers[partitionKey]
}

type fakeRoundTripper struct {
	lock *sync.Mutex
	used bool
}

func (frt *fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	frt.lock.Lock()
	frt.used = true
	frt.lock.Unlock()
	return &http.Response{}, nil
}

func TestPartitioningRoundTripper(t *testing.T) {
	creator := &fakeRoundTripperCreator{
		t:             t,
		lock:          &sync.Mutex{},
		roundTrippers: map[string]*fakeRoundTripper{},
	}
	partitioningRoundTripper := newPartitioningRoundTripper(creator.createRoundTripper)

	requests := []*http.Request{
		{Header: http.Header(map[string][]string{"Authorization": {"a"}})},
		{Header: http.Header(map[string][]string{"Authorization": {"a"}})},
		{Header: http.Header(map[string][]string{"Authorization": {"b"}})},
		{Header: http.Header(map[string][]string{"Authorization": {"b"}})},
		{Header: http.Header(map[string][]string{"Authorization": {"c"}})},
		{Header: http.Header(map[string][]string{"Authorization": {"c"}})},
	}

	// Do these in parallel to verify thread safety
	wg := &sync.WaitGroup{}
	for _, request := range requests {
		wg.Add(1)
		request := request
		go func() {
			defer wg.Done()
			_, err := partitioningRoundTripper.RoundTrip(request)
			if err != nil {
				t.Errorf("RoundTrip: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := len(creator.roundTrippers); n != 3 {
		t.Errorf("expected three roundtrippers, got %d (%v)", n, creator.roundTrippers)
	}
	for name, roundTripper := range creator.roundTrippers {
		if !roundTripper.used {
			t.Errorf("roundtripper %q wasnt used", name)
		}
	}
}
