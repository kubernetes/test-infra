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
	"bufio"
	"bytes"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/ghproxy/ghmetrics"
)

// requestCoalescer allows concurrent requests for the same URI to share a
// single upstream request and response. Once a request comes in for processing
// for the first time, it is processed and a response is received (via
// "requestExecutor"). Meanwhile, if there are any other requests for the same URI,
// those threads Wait(). Then when the first request is done processing (we
// receive a real request), we copy the original request's response into the
// subscribed threads, before letting them all finish. The "cache" map is there
// for our own short-term memory of knowing which request is the "first" one of
// its kind.
type requestCoalescer struct {
	sync.Mutex
	cache map[string]*firstRequest

	// requestExecutor is anything that can resolve a request by executing a
	// single HTTP transaction, returning a Response for the provided Request.
	// The coalescer uses this to talk to the actual proxied backend. Using an
	// interface here allows us to mock out a fake backend server's response to
	// the request.
	requestExecutor http.RoundTripper

	hasher ghmetrics.Hasher
}

// firstRequest is where we store the coalesced requests's actual response. It
// is named firstRequest because only the first one (which also creates the
// entry in the cache) will actually be resolved by being processed over the
// network; all subsequent requests that match the first request's URL will end
// up waiting for this first request to finish. After the first request is
// processed, the "resp" field will be populated, and subsequent requests will
// simply reuse the same "resp" body. Note that if the first request fails, then
// all subsequent requests will fail together.
type firstRequest struct {
	*sync.Cond

	// How many other threads are "subscribed" to this first request's
	// response?
	subscribers int
	resp        []byte
	err         error
}

// RoundTrip coalesces concurrent GET requests for the same URI by blocking
// the later requests until the first request returns and then sharing the
// response between all requests.
func (coalescer *requestCoalescer) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only coalesce GET requests
	if req.Method != http.MethodGet {
		resp, err := coalescer.requestExecutor.RoundTrip(req)
		if strings.HasPrefix(req.URL.Path, "graphql") || strings.HasPrefix(req.URL.Path, "/graphql") {
			var tokenBudgetName string
			if val := req.Header.Get(TokenBudgetIdentifierHeader); val != "" {
				tokenBudgetName = val
			} else {
				tokenBudgetName = coalescer.hasher.Hash(req)
			}
			collectMetrics(ModeNoStore, req, resp, tokenBudgetName)
		}
		return resp, err
	}

	var cacheMode = ModeError
	resp, err := func() (*http.Response, error) {
		key := req.URL.String()
		coalescer.Lock()
		firstReq, ok := coalescer.cache[key]
		// Note that we cannot immediately Unlock() coalescer here just after
		// the cache lookup, because that may result in multiple threads
		// possibly becoming a "firstReq" creator (main) thread. This is why we
		// only Unlock() coalescer __after__ creating the cache entry.

		// Earlier request in flight. Wait for its response, which will be
		// received by a different thread (specifically, the original thread
		// that created the firstReq object --- let's call this the "main"
		// thread for simplicity).
		if ok {
			// Unlock the coalescer, so that other threads can read from it.
			// That is, the coalescer itself should never be blocked by
			// subscribed threads.
			coalescer.Unlock()

			// Let the main thread know that there is at least one subscriber (us).
			firstReq.L.Lock()
			firstReq.subscribers++

			// The documentation for Wait() says:
			// "Because c.L is not locked when Wait first resumes, the caller typically
			// cannot assume that the condition is true when Wait returns. Instead, the
			// caller should Wait in a loop."
			// This does not apply to this use of Wait() because the condition we are
			// waiting for remains true once it becomes true. This lets us avoid the
			// normal check to see if the condition has switched back to false between
			// the signal being sent and this thread acquiring the lock.

			// Unlock firstReq.L variable (so that the thread that __did__ create
			// the first request can actually process it). Suspend execution of
			// this thread until that is done.
			firstReq.Wait()

			// Because firstReq.Wait() will lock firstReq.L before returning,
			// release the lock now because we won't be modifying anything
			// inside firstRequest. Anyway, firstRequest has now completed and
			// we can read from it.
			firstReq.L.Unlock()

			if firstReq.err != nil {
				// Don't log the error ourselves, because it will be logged once
				// by the main thread. This avoids spamming the logs with the
				// same error.
				return nil, firstReq.err
			}

			// Copy in firstReq's response into our own response. We didn't have
			// to process the request ourselves! Wasn't that easy?
			resp, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(firstReq.resp)), nil)
			if err != nil {
				logrus.WithField("cache-key", key).WithError(err).Error("Error loading response.")
				return nil, err
			}

			cacheMode = ModeCoalesced
			return resp, nil
		}

		// No earlier (first) request in flight yet. Create a new firstRequest
		// object and process it ourselves.
		firstReq = &firstRequest{Cond: sync.NewCond(&sync.Mutex{})}
		coalescer.cache[key] = firstReq

		// Unlock the coalescer so that it doesn't block on this particular
		// request. This allows subsequent requests for the same URL to become
		// subscribers to this main one.
		coalescer.Unlock()

		// Actually process the request and get a response.
		resp, err := coalescer.requestExecutor.RoundTrip(req)

		// Real response received. Remove this firstRequest from the cache first
		// __before__ waking any subscribed threads to let them copy the
		// response we got. This order is important. If delete the cache entry
		// __after__ waking the subscribed threads, then the following race
		// condition can happen:
		//
		//  1. firstReq creator thread wakes subscribed threads
		//  2. subscribed threads begin copying data from firstReq struct
		//  3. *NEW* subscribers get created, because the cached key is still there
		//  4. cached key is finally deleted
		//  5. firstReq creator thread from Step 1 dies
		//  6. subscribed threads from Step 3 will wait forever
		//     (memory leak, not to mention request timeout for all of these)
		//
		// Deleting the cache key now also allows a new firstRequest{} object to
		// be created (and the whole cycle repeated again) by another set of
		// requests in flight, if any.
		coalescer.Lock()
		delete(coalescer.cache, key)
		coalescer.Unlock()

		// Write response data into firstReq for all subscribers to see. But
		// only bother with writing into firstReq if we have subscribers at all
		// (because otherwise no other thread will use it anyway).
		firstReq.L.Lock()
		if firstReq.subscribers > 0 {
			if err != nil {
				firstReq.resp, firstReq.err = nil, err
			} else {
				// Copy the response into firstReq.resp before letting
				// subscribers know about it.
				firstReq.resp, firstReq.err = httputil.DumpResponse(resp, true)
			}

			// Wake up all subscribed threads. They will all read firstReq.resp
			// to construct their own (identical) HTTP Responses, based on the
			// contents of firstReq.
			firstReq.Broadcast()
		}
		firstReq.L.Unlock()

		// The RoundTrip() encountered an error. Log it.
		if err != nil {
			logrus.WithField("cache-key", key).WithError(err).Warn("Error from cache transport layer.")
			return nil, err
		}

		// Return a ModeMiss by default (that is, the response was not in the
		// cache, so we had to proxy the request and cache the response). This
		// is what cacheResponseMode() does, unless there are other modes we can
		// glean from the response header, find it with cacheResponseMode.
		cacheMode = cacheResponseMode(resp.Header)

		return resp, nil
	}()

	var tokenBudgetName string
	if val := req.Header.Get(TokenBudgetIdentifierHeader); val != "" {
		tokenBudgetName = val
	} else {
		tokenBudgetName = coalescer.hasher.Hash(req)
	}

	collectMetrics(cacheMode, req, resp, tokenBudgetName)
	return resp, err
}

func collectMetrics(cacheMode CacheResponseMode, req *http.Request, resp *http.Response, tokenBudgetName string) {
	ghmetrics.CollectCacheRequestMetrics(string(cacheMode), req.URL.Path, req.Header.Get("User-Agent"), tokenBudgetName)
	if resp != nil {
		resp.Header.Set(CacheModeHeader, string(cacheMode))
		if cacheMode == ModeRevalidated && resp.Header.Get(cacheEntryCreationDateHeader) != "" {
			intVal, err := strconv.Atoi(resp.Header.Get(cacheEntryCreationDateHeader))
			if err != nil {
				logrus.WithError(err).WithField("header-value", resp.Header.Get(cacheEntryCreationDateHeader)).Warn("Failed to convert cacheEntryCreationDateHeader value to int")
			} else {
				ghmetrics.CollectCacheEntryAgeMetrics(float64(time.Now().Unix()-int64(intVal)), req.URL.Path, req.Header.Get("User-Agent"), tokenBudgetName)
			}
		}
	}
}
