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
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/ghproxy/ghmetrics"

	"k8s.io/apimachinery/pkg/util/diff"
)

const fakeGitHubDomain string = "http://fake-github.com"

// fakeRequestExecutor is a fake upstream transport RoundTripper that logs hits by
// endpoint. It will wait to respond to requests until signaled, or respond
// immediately if the request has a header specifying it should be responded to
// immediately.
type fakeRequestExecutor struct {
	hitsLock sync.Mutex
	hits     map[string]int

	responseHeader     http.Header
	finishFirstRequest chan bool
}

// RoundTrip can generate a fake HTTP response, and can also record the response
// by keeping state in the fakeRequestExecutor.
func (fre *fakeRequestExecutor) RoundTrip(req *http.Request) (*http.Response, error) {
	fre.hitsLock.Lock()
	fre.hits[req.URL.Path] += 1
	fre.hitsLock.Unlock()

	// Construct the fake HTTP response.
	header := fre.responseHeader
	if header == nil {
		header = http.Header{}
	}

	// Block until we're told to finish creating the first request's response.
	// We rely on the requestCoalescer to only call us for the
	// first request.
	<-fre.finishFirstRequest
	return &http.Response{
			Body:   ioutil.NopCloser(bytes.NewBufferString("Response")),
			Header: header,
		},
		nil
}

// concurrentRequestGroup describes a single URL path endpoint (minus the
// domain) and how many total concurrent requests should be made against it.
type concurrentRequestGroup struct {
	endpoint string
	size     int
}

// spawn creates a sudden burst of multiple concurrent requests for multiple
// endpoints.
func spawnGroups(
	crgs []concurrentRequestGroup,
	t *testing.T,
	coalescer *requestCoalescer,
	wg *sync.WaitGroup) {

	for _, concurrentRequestGroup := range crgs {
		concurrentRequestGroup.spawn(t, coalescer, wg)
	}
}

// spawn creates a burst of multiple concurrent requets for a single endpoint.
func (crg concurrentRequestGroup) spawn(
	t *testing.T,
	coalescer *requestCoalescer,
	wg *sync.WaitGroup) {

	wg.Add(crg.size)
	for i := 0; i < crg.size; i++ {
		go func() {
			if _, err := runRequest(coalescer, crg.endpoint); err != nil {
				t.Errorf("Failed to run request: %v.", err)
			}
			wg.Done()
		}()
	}
}

// runRequest creates an HTTP Request, and resolves it using the given HTTP
// RoundTripper and finally returns an HTTP Response. In production we use the
// requestCoalescer to essentially cache multiple requests to GitHub into a
// single request. The runRequest function acts as a "fake GitHub" because we
// can get responses back immediately without going over the network.
func runRequest(coalescer *requestCoalescer, endpoint string) (*http.Response, error) {

	// Construct an HTTP request.
	u, err := url.Parse(fakeGitHubDomain + endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// Squelch missing Authorization header warning from requestCoalescer.hasher.
	req.Header.Set("Authorization", "unknown")

	// Construct an HTTP response for the given request.
	var resp *http.Response

	resp, err = coalescer.RoundTrip(req)
	if err == nil {
		if b, readErr := ioutil.ReadAll(resp.Body); readErr != nil {
			err = readErr
		} else if string(b) != "Response" {
			err = errors.New("unexpected response value")
		}
	}

	return resp, err
}

// waitUntilCacheIsFull waits until all concurrentRequestGroups are full.
func waitUntilCacheIsFull(
	concurrentRequestGroups []concurrentRequestGroup,
	coalescer *requestCoalescer) {

	// Look at each concurrentRequestGroup, and only check it off if we see
	// size-1 subscribed goroutines for it in the coalescer.
	for _, concurrentRequestGroup := range concurrentRequestGroups {
		waitUntilCacheIsFullForGroup(concurrentRequestGroup, coalescer)
	}
}

func waitUntilCacheIsFullForGroup(
	concurrentRequestGroup concurrentRequestGroup,
	coalescer *requestCoalescer) {

	// Wait for the cache entry for this group's endpoint to appear in the
	// coalescer.
	firstReq := waitUntilCacheKeyCreation(fakeGitHubDomain+concurrentRequestGroup.endpoint, coalescer)

	// Wait for this concurrentRequestGroup's requests to be fully loaded
	// into the coalescer's cache. Notice that we never hold both the outer
	// coalescer lock and the inner firstReq lock at the same time. If we do
	// hold both locks, we may deadlock because the requestCoalescer's main
	// thread (which does the actual HTTP round trip) may get stuck trying
	// to lock both locks one after the other (which will never complete if
	// both locks are held by this thread).
	waitingForSubscribers := true
	for waitingForSubscribers {
		// If we only want to create a single request, we don't expect
		// there to be any subscribers.
		if concurrentRequestGroup.size == 1 {
			break
		}

		// There are some pending requests being handled by the coalescer
		// for this particular URL. Watch subscribers in a busy loop until
		// we see the expected number of subscribed threads.
		for {
			firstReq.L.Lock()
			subscribers := firstReq.subscribers
			firstReq.L.Unlock()
			// If there are size - 1 subscribed threads, then the coalescer's
			// cache is fully loaded because there are no more subscribers that
			// are scheduled to arrive.
			if subscribers == (concurrentRequestGroup.size - 1) {
				waitingForSubscribers = false
				break
			}
		}
	}
}

// waitUntilCacheKeyCreation waits until the coalescer has created a cache
// entry. The cache entry is only created by the first thread that reaches the
// coalescer (reverse proxy, in this case, to the fake GitHub server). This is
// useful for waiting until a goroutine has been officially "promoted" to be the
// first thread that has reached the coalescer, responsible for invoking the
// RoundTripper.
func waitUntilCacheKeyCreation(
	key string,
	coalescer *requestCoalescer) *firstRequest {

	for {
		coalescer.Lock()
		firstReq, keyExists := coalescer.cache[key]
		if keyExists {
			coalescer.Unlock()
			return firstReq
		}
		coalescer.Unlock()
	}
}

// waitUntilCacheKeyDeletion waits until the coalescer has deleted the given
// cache entry. This is useful for detecting when an initial burst of concurrent
// requests to a single endpoint has subsided.
func waitUntilCacheKeyDeletion(
	key string,
	coalescer *requestCoalescer) {

	for {
		coalescer.Lock()
		_, keyExists := coalescer.cache[key]
		if !keyExists {
			coalescer.Unlock()
			break
		}
		coalescer.Unlock()
	}
}

// TestRoundTrip checks that only 1 request goes to upstream if there are
// concurrent requests for the same URL.
func TestRoundTrip(t *testing.T) {
	fre := &fakeRequestExecutor{
		hits:               make(map[string]int),
		finishFirstRequest: make(chan bool),
	}
	coalescer := &requestCoalescer{
		cache:           make(map[string]*firstRequest),
		requestExecutor: fre,
		hasher:          ghmetrics.NewCachingHasher(),
	}

	// Create 500 fake requests, 100 for each unique URL. For each group, we
	// expect 1 of them to be the first one, and 99 of them to be "subscribed"
	// to the first one (for the first one to finish). For 99 of these
	// goroutines, we expect them to wait because of the coalescer.
	concurrentRequestGroups := []concurrentRequestGroup{
		{"/resource1",
			100,
		},
		{"/resource2",
			100,
		},
		{"/resource3",
			100,
		},
		{"/resource4",
			100,
		},
		{"/resource5",
			100,
		},
	}

	wg := sync.WaitGroup{}

	// We need to wait for all requests to be made to the coalescer. The calls
	// to spawnGroups() and waitUntilCacheIsFull() establish a state where the
	// coalescer is still holding onto __all__ pending concurrent requests. None
	// of the requests we've made above have finished yet because we have not
	// told the fake GitHub server to actually respond yet. That's what we do
	// below.
	spawnGroups(concurrentRequestGroups, t, coalescer, &wg)
	waitUntilCacheIsFull(concurrentRequestGroups, coalescer)

	// We only made 5 unique requests to the fake GitHub server. Tell the fake
	// GitHub to finally respond to these 5 requests. This triggers a chain
	// reaction where:
	//
	//  1. (inside requestCoalescer) the lucky threads that got the firstRequest
	//  finally receive (through our fakeRequestExecutor) a response from the
	//  fake GitHub server, and
	//
	//  2. (inside requestCoalescer) the subscribed threads that were waiting
	//  for firstRequest to finish being populated also get to finish
	//  constructing their own HTTP Response bodies and return.
	//
	// The above 2 steps happen concurrently for all 5 coalesced requests.
	for i := 0; i < len(concurrentRequestGroups); i++ {
		fre.finishFirstRequest <- true
	}

	// Check that non-concurrent requests all hit upstream. We simulate this by
	// creating another round of concurrent requests, but with size 1.
	concurrentRequestGroups2 := []concurrentRequestGroup{
		{"/resource2",
			1,
		},
	}
	// Wait until the lucky thread that handled the "/resource2" endpoint in the
	// first group of concurrent requests has finished running (deleted the
	// cache entry). This is important because otherwise the thread spawned
	// below might (due to a race) get wrongly categorized as another
	// "subscriber" thread as part of the concurrent requests above. Forcing
	// this main thread to wait until the original cached entry is deleted
	// guarantees that the request below will be handled as a new key.
	waitUntilCacheKeyDeletion(fakeGitHubDomain+"/resource2", coalescer)
	spawnGroups(concurrentRequestGroups2, t, coalescer, &wg)
	fre.finishFirstRequest <- true
	wg.Wait()

	expectedHits := map[string]int{
		"/resource1": 1,
		"/resource2": 2,
		"/resource3": 1,
		"/resource4": 1,
		"/resource5": 1,
	}
	if !reflect.DeepEqual(fre.hits, expectedHits) {
		t.Errorf("Unexpected hit count(s). Diff: %v.", diff.ObjectReflectDiff(expectedHits, fre.hits))
	}
}

// TestRoundTripSynthetic runs until we've processed 100,000 requests. During
// this time, we create an upper limit of 1000 concurrent requests to 10
// different URL endpoints in flight at any given time. Meanwhile, the fake
// GitHub server randomly takes between 1 or 5 milliseconds to respond to each
// proxied request. We simply expect this test to finish, and that all requests
// are served (no timeouts).
func TestRoundTripSynthetic(t *testing.T) {
	// Detect deadlocks. We should be able to finish this test in 10 seconds.
	timeout := time.After(10 * time.Second)
	done := make(chan bool)
	shutdownFakeGitHub := make(chan bool)

	go func() {
		fre := &fakeRequestExecutor{
			hits:               make(map[string]int),
			finishFirstRequest: make(chan bool),
		}
		coalescer := &requestCoalescer{
			cache:           make(map[string]*firstRequest),
			requestExecutor: fre,
			hasher:          ghmetrics.NewCachingHasher(),
		}

		synthetic := sync.Mutex{}

		const maxConcurrentRequests = 1_000
		concurrentRequests := 0

		const maxStartedRequests = 100_000
		startedRequests := 0

		rand.Seed(time.Now().Unix())
		endpoints := []string{
			"/resource/1",
			"/resource/2",
			"/resource/3",
			"/resource/4",
			"/resource/5",
			"/resource/6",
			"/resource/7",
			"/resource/8",
			"/resource/9",
			"/resource/10",
		}

		// From the fake GitHub server side, complete requests after sleeping a
		// random interval. Stop responding when we know that we've processed
		// 100,000 requests in total.
		go func() {
			fakeGitHubOnline := true
			for fakeGitHubOnline {
				select {
				case <-shutdownFakeGitHub:
					close(fre.finishFirstRequest)
					fakeGitHubOnline = false
				default:
					time.Sleep(time.Duration(rand.Intn(5)+1) * time.Millisecond)
					fre.finishFirstRequest <- true
				}
			}
		}()

		for {
			synthetic.Lock()
			// We've created all 100,000 requests we wanted to create so far, so
			// we're done with this test.
			if startedRequests == maxStartedRequests {
				synthetic.Unlock()
				shutdownFakeGitHub <- true
				break
			}
			if concurrentRequests == maxConcurrentRequests {
				synthetic.Unlock()
				continue
			}

			startedRequests++
			concurrentRequests++
			synthetic.Unlock()

			go func() {
				randomEndpoint := endpoints[rand.Intn(len(endpoints))]
				if _, err := runRequest(coalescer, randomEndpoint); err != nil {
					t.Errorf("Failed to run request: %v.", err)
				}
				synthetic.Lock()
				concurrentRequests--
				synthetic.Unlock()
			}()
		}

		// Show some diagnostics.
		logrus.Infof("TestRoundTripSyntehtic: fre.hits: '%v'", fre.hits)

		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("Deadlocked")
	case <-done:
	}
}

func TestCacheModeHeader(t *testing.T) {
	wg := sync.WaitGroup{}
	fre := &fakeRequestExecutor{
		hits:               make(map[string]int),
		finishFirstRequest: make(chan bool),
	}
	coalescer := &requestCoalescer{
		cache:           make(map[string]*firstRequest),
		requestExecutor: fre,
		hasher:          ghmetrics.NewCachingHasher(),
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
		if resp, err := runRequest(coalescer, "/resource1"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeMiss)
		}
		wg.Done()
	}()

	// Wait until a cache entry is created. This ensures that the goroutine
	// above will end up as the one performing the RoundTrip.
	waitUntilCacheKeyCreation(fakeGitHubDomain+"/resource1", coalescer)

	// Queue a second request for resource1.
	// This should coalescer and eventually return ModeCoalesced.
	crg := []concurrentRequestGroup{
		{"/resource1",
			2,
		},
	}
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalescer, "/resource1"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeCoalesced)
		}
		wg.Done()
	}()
	// Now there are 2 requests pending --- the first one that received
	// "/resource1", and the second one that is also reqeusting "/resource1".
	// Wait to ensure that they are both in flight the requestCoalescer.
	waitUntilCacheIsFull(crg, coalescer)

	// Requests should be waiting now. Make fake GitHub finally respond. We only
	// created 1 unique URL, so we only need to unblock 1 thread that will
	// perform the actual RoundTrip; and so we only pass in a single boolean to
	// fre.finishFirstRequest.
	fre.finishFirstRequest <- true
	wg.Wait()

	// A later request for resource1 revalidates cached response.
	// This should return ModeRevalidated.
	header := http.Header{}
	header.Set("Status", "304 Not Modified")
	fre.responseHeader = header
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalescer, "/resource1"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeRevalidated)
		}
		wg.Done()
	}()
	fre.finishFirstRequest <- true
	wg.Wait()

	// Another request for resource1 after the resource has changed.
	// This should return ModeChanged.
	header = http.Header{}
	header.Set("X-Conditional-Request", "I am an E-Tag.")
	fre.responseHeader = header
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalescer, "/resource1"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeChanged)
		}
		wg.Done()
	}()
	fre.finishFirstRequest <- true
	wg.Wait()

	// Request for new resource2 with no concurrent requests.
	// This should return ModeMiss.
	fre.responseHeader = nil
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalescer, "/resource2"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeMiss)
		}
		wg.Done()
	}()
	fre.finishFirstRequest <- true
	wg.Wait()

	// Request for uncacheable resource3.
	// This should return ModeNoStore.
	header = http.Header{}
	header.Set("Cache-Control", "no-store")
	fre.responseHeader = header
	wg.Add(1)
	go func() {
		if resp, err := runRequest(coalescer, "/resource3"); err != nil {
			t.Errorf("Failed to run request: %v.", err)
		} else {
			checkMode(resp, ModeNoStore)
		}
		wg.Done()
	}()
	fre.finishFirstRequest <- true
	wg.Wait()

	// We never send a ModeError mode in a header because we never return a
	// http.Response if there is an error. ModeError is only for metrics.

	// Might as well mind the hit count in this test too.
	expectedHits := map[string]int{"/resource1": 3, "/resource2": 1, "/resource3": 1}
	if !reflect.DeepEqual(fre.hits, expectedHits) {
		t.Errorf("Unexpected hit count(s). Diff: %v.", diff.ObjectReflectDiff(expectedHits, fre.hits))
	}
}

// TestStress runs tests multiple times, to attempt to detect any
// races in our own test code (along with the code being tested). This is
// important as subtle concurrency bugs can only be observed after multiple
// runs.
func TestStress(t *testing.T) {
	for i := 0; i < 100; i++ {
		TestRoundTrip(t)
		TestCacheModeHeader(t)
	}
}
