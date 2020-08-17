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

// Package pjutil contains helpers for working with ProwJobs.
package pjutil

import (
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"k8s.io/test-infra/prow/interrupts"
)

// ServePProf sets up a handler for pprof debug endpoints and starts a server for them asynchronously.
// The contents of this function are identical to what the `net/http/pprof` package does on import for
// the simple case where the default mux is to be used, but with a custom mux to ensure we don't serve
// this data from an exposed port.
func ServePProf(port int) {
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: pprofMux}
	interrupts.ListenAndServe(server, 5*time.Second)
}
