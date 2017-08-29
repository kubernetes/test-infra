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

package sharedmux

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

var (
	// Admin is the schelling point for those who want to provide admin
	// actions and those who want to install the admin portal in some port.
	Admin = NewAdminMux()
)

// ConcurrentMux is safe to call HandleFunc on even after it's started serving
// requests.
type ConcurrentMux struct {
	mux      *http.ServeMux
	pathList []string
	lock     sync.RWMutex
}

// NewConcurrentMux constructs a mux.
func NewConcurrentMux(mux *http.ServeMux) *ConcurrentMux {
	return &ConcurrentMux{
		mux: mux,
	}
}

func NewAdminMux() *ConcurrentMux {
	c := NewConcurrentMux(http.NewServeMux())
	c.Handle("/", http.HandlerFunc(c.listHTTP))
	return c
}

// Handle installs the given handler.
func (c *ConcurrentMux) Handle(pattern string, handler http.Handler) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.mux.Handle(pattern, handler)
	c.pathList = append(c.pathList, pattern)
}

// HandleFunc installs the given handler function.
func (c *ConcurrentMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	c.Handle(pattern, http.HandlerFunc(handler))
}

// ServeHTTP serves according to the added handlers.
func (c *ConcurrentMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	c.mux.ServeHTTP(w, r)
}

// listHTTP lists handlers that have been added.
func (c *ConcurrentMux) listHTTP(w http.ResponseWriter, r *http.Request) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	fmt.Fprintf(w, "Possible paths:\n%v\n", strings.Join(c.pathList, "\n"))
}
