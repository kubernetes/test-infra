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
// single upstream request and response.
type requestCoalescer struct {
	sync.Mutex
	cache map[string]*firstRequest

	requestExecutor http.RoundTripper

	hasher ghmetrics.Hasher
}

type firstRequest struct {
	*sync.Cond

	subscribers bool
	resp        []byte
	err         error
}

// RoundTrip coalesces concurrent GET requests for the same URI by blocking
// the later requests until the first request returns and then sharing the
// response between all requests.
//
// Notes: Deadlock shouldn't be possible because the map lock is always
// acquired before responseWaiter lock if both locks are to be held and we
// never hold multiple responseWaiter locks.
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
		if ok {
			// Earlier request in flight. Wait for it's response.
			if req.Body != nil {
				defer req.Body.Close() // Since we won't pass the request we must close it.
			}
			firstReq.L.Lock()
			coalescer.Unlock()
			firstReq.subscribers = true

			// The documentation for Wait() says:
			// "Because c.L is not locked when Wait first resumes, the caller typically
			// cannot assume that the condition is true when Wait returns. Instead, the
			// caller should Wait in a loop."
			// This does not apply to this use of Wait() because the condition we are
			// waiting for remains true once it becomes true. This lets us avoid the
			// normal check to see if the condition has switched back to false between
			// the signal being sent and this thread acquiring the lock.
			firstReq.Wait()
			firstReq.L.Unlock()

			if firstReq.err != nil {
				// Don't log the error, it will be logged by requester.
				return nil, firstReq.err
			}
			resp, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(firstReq.resp)), nil)
			if err != nil {
				logrus.WithField("cache-key", key).WithError(err).Error("Error loading response.")
				return nil, err
			}

			cacheMode = ModeCoalesced
			return resp, nil
		}
		// No earlier request in flight (common case).
		// Register a new responseWaiter and make the request ourself.
		firstReq = &firstRequest{Cond: sync.NewCond(&sync.Mutex{})}
		coalescer.cache[key] = firstReq
		coalescer.Unlock()

		resp, err := coalescer.requestExecutor.RoundTrip(req)
		// Real response received. Remove this responseWaiter from the map THEN
		// wake any requesters that were waiting on this response.
		coalescer.Lock()
		delete(coalescer.cache, key)
		coalescer.Unlock()

		firstReq.L.Lock()
		if firstReq.subscribers {
			if err != nil {
				firstReq.resp, firstReq.err = nil, err
			} else {
				// Copy the response before releasing to waiter(s).
				firstReq.resp, firstReq.err = httputil.DumpResponse(resp, true)
			}
			firstReq.Broadcast()
		}
		firstReq.L.Unlock()

		if err != nil {
			logrus.WithField("cache-key", key).WithError(err).Warn("Error from cache transport layer.")
			return nil, err
		}
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
