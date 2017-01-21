/*
Copyright 2016 The Kubernetes Authors.

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

package e2e

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	cache "k8s.io/test-infra/mungegithub/mungers/flakesync"

	"github.com/golang/glog"
)

type resolutionKey struct {
	job    cache.Job
	number cache.Number
}

// ResolutionTracker provides a place for build cops to say "I've resolved this
// problem" so the merge queue can continue merging.
type ResolutionTracker struct {
	resolved map[resolutionKey]bool
	lock     sync.RWMutex
}

// NewResolutionTracker constructs an empty resolution tracker. It's the
// caller's responsibility to hook up Get/SetHTTP.
func NewResolutionTracker() *ResolutionTracker {
	return &ResolutionTracker{
		resolved: map[resolutionKey]bool{},
	}
}

// Resolved returns true if the given build has been manually marked as resolved.
func (r *ResolutionTracker) Resolved(j cache.Job, n cache.Number) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.resolved[resolutionKey{j, n}]
}

func getReqKey(req *http.Request) (key resolutionKey, err error) {
	key.job = cache.Job(req.URL.Query().Get("job"))
	var n int
	n, err = strconv.Atoi(req.URL.Query().Get("number"))
	key.number = cache.Number(n)
	return key, err
}

func (r *ResolutionTracker) serve(data interface{}, status int, res http.ResponseWriter) {
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(status)
	if err := json.NewEncoder(res).Encode(data); err != nil {
		glog.Errorf("Couldn't write %#v: %v", data, err)
	}
}

// ListHTTP returns a list of overrides that have been entered.
func (r *ResolutionTracker) ListHTTP(res http.ResponseWriter, req *http.Request) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	type Entry struct {
		Job      cache.Job
		Number   cache.Number
		Resolved bool
	}
	var list []Entry
	for key, resolved := range r.resolved {
		if !resolved {
			continue
		}
		list = append(list, Entry{
			Job:      key.job,
			Number:   key.number,
			Resolved: true,
		})
	}
	r.serve(struct{ ManualResolutions []Entry }{list}, http.StatusOK, res)
}

// GetHTTP accepts "job" and "number" query parameters and returns a json blob
// indicating whether the specified build has been marked as resolved.
func (r *ResolutionTracker) GetHTTP(res http.ResponseWriter, req *http.Request) {
	key, err := getReqKey(req)
	if err != nil {
		r.serve(struct{ Error string }{err.Error()}, http.StatusNotAcceptable, res)
		return
	}
	r.lock.RLock()
	defer r.lock.RUnlock()
	r.serveKeyLocked(key, res)
}

// SetHTTP accepts "job", "number", and "resolved" query parameters. "resolved"
// counts as "true" if not explicitly set to "false". Returns a json blob
// indicating whether the specified build has been marked as resolved.
func (r *ResolutionTracker) SetHTTP(res http.ResponseWriter, req *http.Request) {
	key, err := getReqKey(req)
	if err != nil {
		r.serve(struct{ Error string }{err.Error()}, http.StatusNotAcceptable, res)
		return
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.resolved[key] = req.URL.Query().Get("resolved") != "false"
	glog.Infof("Marking manually resolved: %v %v: %v", key.job, key.number, r.resolved[key])
	r.serveKeyLocked(key, res)
}

func (r *ResolutionTracker) serveKeyLocked(key resolutionKey, res http.ResponseWriter) {
	r.serve(struct {
		Job      string
		Number   int
		Resolved bool
	}{
		string(key.job),
		int(key.number),
		r.resolved[key],
	}, http.StatusOK, res)
}
