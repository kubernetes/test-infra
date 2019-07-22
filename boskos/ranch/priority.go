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
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// request stores request information with expiration
type request struct {
	id         string
	expiration time.Time
}

func newRequestQueue() *requestQueue {
	return &requestQueue{
		requestMap: map[string]request{},
	}
}

// requestQueue is a simple FIFO queue for requests.
type requestQueue struct {
	lock        sync.RWMutex
	requestList []string
	requestMap  map[string]request
}

// update updates expiration time is updated if already present,
// add a new requestID at the end otherwise (FIFO)
func (rq *requestQueue) update(requestID string, newExpiration time.Time) {
	if requestID == "" {
		// Nothing to do
		return
	}
	rq.lock.Lock()
	defer rq.lock.Unlock()
	if rq.requestMap == nil {
		rq.requestMap = map[string]request{}
	}
	req, exists := rq.requestMap[requestID]
	if !exists {
		req = request{id: requestID}
		rq.requestList = append(rq.requestList, requestID)
		logrus.Infof("request id %s added", requestID)
	}
	// Update timestamp
	req.expiration = newExpiration
	rq.requestMap[requestID] = req
	logrus.Infof("request id %s set to expire at %v", requestID, newExpiration)
}

// delete an element
func (rq *requestQueue) delete(requestID string) {
	rq.lock.Lock()
	defer rq.lock.Unlock()
	delete(rq.requestMap, requestID)
	index := -1
	for i, id := range rq.requestList {
		if id == requestID {
			index = i
			break
		}
	}
	if index >= 0 {
		rq.requestList = append(rq.requestList[0:index], rq.requestList[index+1:len(rq.requestList)]...)
		logrus.Infof("request id %s deleted", requestID)
	}
}

// cleanup checks for all expired  or marked for deletion items and delete them.
func (rq *requestQueue) cleanup(now time.Time) {
	rq.lock.Lock()
	defer rq.lock.Unlock()
	var newRequestList []string
	newRequestMap := map[string]request{}
	for _, requestID := range rq.requestList {
		req := rq.requestMap[requestID]
		// Checking expiration
		if now.After(req.expiration) {
			logrus.Infof("request id %s expired", req.id)
			continue
		}
		// Keeping
		newRequestList = append(newRequestList, requestID)
		newRequestMap[requestID] = req
	}
	rq.requestMap = newRequestMap
	rq.requestList = newRequestList
}

// getRank provides the rank of a given requestID following the order it was added (FIFO).
// If requestID is an empty string, getRank assumes it is added last (lowest rank + 1).
func (rq *requestQueue) getRank(requestID string, ttl time.Duration, now time.Time) int {
	rq.update(requestID, now.Add(ttl))
	rank := 1
	rq.lock.RLock()
	defer rq.lock.RUnlock()
	for _, existingID := range rq.requestList {
		req := rq.requestMap[existingID]
		if now.After(req.expiration) {
			logrus.Infof("request id %s expired", req.id)
			continue
		}
		if requestID == existingID {
			return rank
		}
		rank++
	}
	return rank
}

func (rq *requestQueue) isEmpty() bool {
	rq.lock.Lock()
	defer rq.lock.Unlock()
	return len(rq.requestList) == 0
}

// RequestManager facilitates management of RequestQueues for a set of (resource type, resource state) tuple.
type RequestManager struct {
	lock     sync.Mutex
	requests map[interface{}]*requestQueue
	ttl      time.Duration
	stopGC   context.CancelFunc
	wg       sync.WaitGroup
	// For testing only
	now func() time.Time
}

// NewRequestManager creates a new RequestManager
func NewRequestManager(ttl time.Duration) *RequestManager {
	return &RequestManager{
		requests: map[interface{}]*requestQueue{},
		ttl:      ttl,
		now:      time.Now,
	}
}

func (rp *RequestManager) cleanup(now time.Time) {
	rp.lock.Lock()
	defer rp.lock.Unlock()
	for key, rq := range rp.requests {
		logrus.Infof("cleaning up %v request queue", key)
		rq.cleanup(now)
		if rq.isEmpty() {
			delete(rp.requests, key)
		}
	}
}

// StartGC starts a goroutine that will call cleanup every gcInterval
func (rp *RequestManager) StartGC(gcPeriod time.Duration) {
	ctx, stop := context.WithCancel(context.Background())
	rp.stopGC = stop
	tick := time.Tick(gcPeriod)
	go func() {
		logrus.Info("starting cleanup go routine")
		defer logrus.Info("exiting cleanup go routine")
		rp.wg.Add(1)
		defer rp.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				rp.cleanup(rp.now())
			}
		}

	}()
}

// StopGC is a blocking call that will stop the GC goroutine.
func (rp *RequestManager) StopGC() {
	if rp.stopGC != nil {
		rp.stopGC()
		rp.wg.Wait()
	}
}

// GetRank provides the rank of a given request
func (rp *RequestManager) GetRank(key interface{}, id string) int {
	rp.lock.Lock()
	defer rp.lock.Unlock()
	rq := rp.requests[key]
	if rq == nil {
		rq = newRequestQueue()
		rp.requests[key] = rq
	}
	return rq.getRank(id, rp.ttl, rp.now())
}

// Delete deletes a specific request such that it is not accounted in the next GetRank call.
// This is usually called when the request has been fulfilled.
func (rp *RequestManager) Delete(key interface{}, requestID string) {
	rp.lock.Lock()
	defer rp.lock.Unlock()
	rq := rp.requests[key]
	if rq != nil {
		rq.delete(requestID)
	}
}
