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

// Package admin exists so all administrative actions have a place to install themselves.
package admin

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

var (
	// Mux is the schelling point for those who want to provide admin
	// actions and those who want to install the admin portal in some port.
	Mux = NewConcurrentMux()
)

// ConcurrentMux is safe to call HandleFunc on even after it's started serving
// requests.
type ConcurrentMux struct {
	mux      *http.ServeMux
	pathList []string
	lock     sync.RWMutex
}

// NewConcurrentMux constructs a mux.
func NewConcurrentMux() *ConcurrentMux {
	c := &ConcurrentMux{
		mux: http.NewServeMux(),
	}
	c.HandleFunc("/", c.ListHTTP)
	return c
}

// HandleFunc installs the given handler.
func (c *ConcurrentMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.mux.HandleFunc(pattern, handler)
	c.pathList = append(c.pathList, pattern)
}

// ServeHTTP serves according to the added handlers.
func (c *ConcurrentMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	c.mux.ServeHTTP(w, r)
}

// ListHTTP lists handlers that have been added.
func (c *ConcurrentMux) ListHTTP(w http.ResponseWriter, r *http.Request) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	fmt.Fprintf(w, "Possible paths:\n%v\n", strings.Join(c.pathList, "\n"))
}
