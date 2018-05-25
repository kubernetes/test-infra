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
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

// testDelegate is a fake upstream transport delegate that logs hits by URI and
// will wait to respond to requests until signaled unless the request has
// a header specifying it should be responded to immediately.
type testDelegate struct {
	beginResponding *sync.Cond

	hitsLock sync.Mutex
	hits     map[string]int
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
	return &http.Response{
			Body: ioutil.NopCloser(bytes.NewBufferString("Response")),
		},
		nil
}

func TestRoundTrip(t *testing.T) {
	// Check that only 1 request goes to upstream if there are concurrent requests.
	delegate := &testDelegate{
		hits:            make(map[string]int),
		beginResponding: sync.NewCond(&sync.Mutex{}),
	}
	coalesce := &requestCoalescer{
		keys:     make(map[string]*responseWaiter),
		delegate: delegate,
	}
	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			runRequest(t, coalesce, "/resource1", false)
			wg.Done()
		}()
	}
	// There is a race here. We need to wait for all requests to be made to the
	// coalescer before letting upstream respond, but we don't have a way of
	// knowing when all requests have actually started waiting on the
	// responseWaiter...
	time.Sleep(time.Second * 5)

	// Check that requests for different resources are not blocked.
	runRequest(t, coalesce, "/resource2", true) // Doesn't return until timeout or success.
	delegate.beginResponding.Broadcast()

	// Check that non concurrent requests all hit upstream.
	runRequest(t, coalesce, "/resource2", true)

	wg.Wait()
	expectedHits := map[string]int{"/resource1": 1, "/resource2": 2}
	if !reflect.DeepEqual(delegate.hits, expectedHits) {
		t.Errorf("Unexpected hit count(s). Expected %v, but got %v.", expectedHits, delegate.hits)
	}
}

func runRequest(t *testing.T, rt http.RoundTripper, uri string, immediate bool) {
	res := make(chan error)
	run := func() {
		u, err := url.Parse("http://foo.com" + uri)
		if err != nil {
			res <- err
		}
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			res <- err
		}
		if immediate {
			req.Header.Set("test-immediate-response", "true")
		}
		resp, err := rt.RoundTrip(req)
		if err != nil {
			res <- err
		} else if b, err := ioutil.ReadAll(resp.Body); err != nil {
			res <- err
		} else if string(b) != "Response" {
			res <- errors.New("unexpected response value")
		}
		res <- nil
	}
	go run()
	select {
	case <-time.After(time.Second * 10):
		t.Errorf("Request for %q timed out.", uri)
	case err := <-res:
		if err != nil {
			t.Errorf("Request error: %v.", err)
		}
	}
}
