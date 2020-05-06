/*
Copyright 2018 The Kubernetes Authors.

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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"k8s.io/test-infra/ghproxy/ghmetrics"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/diff"
)

// testDelegate is a fake upstream transport delegate that logs hits by URI and
// will wait to respond to requests until signaled unless the request has
// a header specifying it should be responded to immediately.
type testDelegate struct {
	beginResponding *sync.Cond

	hitsLock sync.Mutex
	hits     map[string]int

	responseHeader http.Header
}

func (t *testDelegate) RoundTrip(req *http.Request) (*http.Response, error) {
	t.hitsLock.Lock()
	t.hits[req.URL.Path] += 1
	t.hitsLock.Unlock()

	if req.Header.Get("test-immediate-response") == "" {
		t.beginResponding.L.Lock()
		t.beginResponding.Wait()
		t.beginResponding.L.Unlock()
	}
	header := t.responseHeader
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
			Body:   ioutil.NopCloser(bytes.NewBufferString("Response")),
			Header: header,
		},
		nil
}

func TestRoundTrip(t *testing.T) {
	// Check that only 1 request goes to upstream if there are concurrent requests.
	t.Parallel()
	delegate := &testDelegate{
		hits:            make(map[string]int),
		beginResponding: sync.NewCond(&sync.Mutex{}),
	}
	coalesce := &requestCoalescer{
		keys:     make(map[string]*responseWaiter),
		delegate: delegate,
		hasher:   ghmetrics.NewCachingHasher(),
	}
	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			if _, err := runRequest(coalesce, "/resource1", false); err != nil {
				t.Errorf("Failed to run request: %v.", err)
			}
			wg.Done()
		}()
	}
	// There is a race here. We need to wait for all requests to be made to the
	// coalescer before letting upstream respond, but we don't have a way of
	// knowing when all requests have actually started waiting on the
	// responseWaiter...
	time.Sleep(time.Second * 5)

	// Check that requests for different resources are not blocked.
	if _, err := runRequest(coalesce, "/resource2", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	} // Doesn't return until timeout or success.
	delegate.beginResponding.Broadcast()

	// Check that non concurrent requests all hit upstream.
	if _, err := runRequest(coalesce, "/resource2", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	}

	wg.Wait()
	expectedHits := map[string]int{"/resource1": 1, "/resource2": 2}
	if !reflect.DeepEqual(delegate.hits, expectedHits) {
		t.Errorf("Unexpected hit count(s). Diff: %v.", diff.ObjectReflectDiff(expectedHits, delegate.hits))
	}
}

func TestCacheModeHeader(t *testing.T) {
	t.Parallel()
	wg := sync.WaitGroup{}
	delegate := &testDelegate{
		hits:            make(map[string]int),
		beginResponding: sync.NewCond(&sync.Mutex{}),
	}
	coalesce := &requestCoalescer{
		keys:     make(map[string]*responseWaiter),
		delegate: delegate,
		hasher:   ghmetrics.NewCachingHasher(),
	}

	checkMode := func(resp *http.Response, expected CacheResponseMode) {
		mode := CacheResponseMode(resp.Header.Get(CacheModeHeader))
		if mode != expected {
			t.Errorf("Expected cache mode %s, but got %s.", string(expected), string(mode))
		}
	}

	// Queue an initial request for resource1.
	// This should eventually return ModeMiss.
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalesce, "/resource1", false); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeMiss)
		}
		wg.Done()
	}()
	// There is a race here and where sleeps are used below.
	// We need to wait for the initial request to be made
	// to the coalescer before letting upstream respond, but we don't have a way
	// of knowing when the requests has actually started waiting on the
	// responseWaiter...
	time.Sleep(time.Second * 3)

	// Queue a second request for resource1.
	// This should coalesce and eventually return ModeCoalesced.
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalesce, "/resource1", false); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeCoalesced)
		}
		wg.Done()
	}()
	time.Sleep(time.Second * 3)

	// Requests should be waiting now. Start responding and wait for all
	// downstream responses to return.
	delegate.beginResponding.Broadcast()
	wg.Wait()

	// A later request for resource1 revalidates cached response.
	// This should return ModeRevalidated.
	header := http.Header{}
	header.Set("Status", "304 Not Modified")
	delegate.responseHeader = header
	if resp, err := runRequest(coalesce, "/resource1", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	} else {
		checkMode(resp, ModeRevalidated)
	}

	// Another request for resource1 after the resource has changed.
	// This should return ModeChanged.
	header = http.Header{}
	header.Set("X-Conditional-Request", "I am an E-Tag.")
	delegate.responseHeader = header
	if resp, err := runRequest(coalesce, "/resource1", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	} else {
		checkMode(resp, ModeChanged)
	}

	// Request for new resource2 with no concurrent requests.
	// This should return ModeMiss.
	delegate.responseHeader = nil
	if resp, err := runRequest(coalesce, "/resource2", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	} else {
		checkMode(resp, ModeMiss)
	}

	// Request for uncacheable resource3.
	// This should return ModeNoStore.
	header = http.Header{}
	header.Set("Cache-Control", "no-store")
	delegate.responseHeader = header
	if resp, err := runRequest(coalesce, "/resource3", true); err != nil {
		t.Errorf("Failed to run request: %v.", err)
	} else {
		checkMode(resp, ModeNoStore)
	}

	// We never send a ModeError mode in a header because we never return a
	// http.Response if there is an error. ModeError is only for metrics.

	// Might as well mind the hit count in this test too.
	expectedHits := map[string]int{"/resource1": 3, "/resource2": 1, "/resource3": 1}
	if !reflect.DeepEqual(delegate.hits, expectedHits) {
		t.Errorf("Unexpected hit count(s). Diff: %v.", diff.ObjectReflectDiff(expectedHits, delegate.hits))
	}
}

func runRequest(rt http.RoundTripper, uri string, immediate bool) (*http.Response, error) {
	u, err := url.Parse("http://foo.com" + uri)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if immediate {
		req.Header.Set("test-immediate-response", "true")
	}

	waitChan := make(chan struct{})
	var resp *http.Response
	go func() {
		defer close(waitChan)
		resp, err = rt.RoundTrip(req)
		if err == nil {
			if b, readErr := ioutil.ReadAll(resp.Body); readErr != nil {
				err = readErr
			} else if string(b) != "Response" {
				err = errors.New("unexpected response value")
			}
		}
	}()

	select {
	case <-time.After(time.Second * 10):
		return nil, fmt.Errorf("Request for %q timed out.", uri)
	case <-waitChan:
		return resp, err
	}
}
