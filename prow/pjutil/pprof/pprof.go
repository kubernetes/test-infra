/*
Copyright 2021 The Kubernetes Authors.

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

// Package pprof contains helpers for profiling binaries.
package pprof

import (
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"runtime"
	runtimepprof "runtime/pprof"
	"strconv"
	"time"

	"github.com/felixge/fgprof"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/flagutil"

	"k8s.io/test-infra/prow/interrupts"
)

// Instrument implements the profiling options a user has asked for on the command line.
func Instrument(opts flagutil.InstrumentationOptions) {
	Serve(opts.PProfPort)
	if opts.ProfileMemory {
		WriteMemoryProfiles(opts.MemoryProfileInterval)
	}
}

// Serve sets up a handler for pprof debug endpoints and starts a server for them asynchronously.
// The contents of this function are identical to what the `net/http/pprof` package does on import for
// the simple case where the default mux is to be used, but with a custom mux to ensure we don't serve
// this data from an exposed port.
func Serve(port int) {
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	pprofMux.Handle("/debug/fgprof", fgprof.Handler())
	server := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: pprofMux}
	interrupts.ListenAndServe(server, 5*time.Second)
}

// WriteMemoryProfiles is a non-blocking, best-effort routine to dump memory profiles at a
// pre-determined interval for future parsing and analysis.
func WriteMemoryProfiles(interval time.Duration) {
	logrus.Info("Writing memory profiles.")
	profileDir, err := ioutil.TempDir("", "heap-profiles-")
	if err != nil {
		logrus.WithError(err).Warn("Could not create a directory to store memory profiles.")
		return
	}
	interrupts.TickLiteral(func() {
		profile, err := ioutil.TempFile(profileDir, "heap-profile-")
		if err != nil {
			logrus.WithError(err).Warn("Could not create a file to store a memory profile.")
			return
		}
		logrus.Info("Writing a memory profile.")
		runtime.GC() // ensure we have up-to-date data
		if err := runtimepprof.WriteHeapProfile(profile); err != nil {
			logrus.WithError(err).Warn("Could not write memory profile.")
		}
		logrus.Infof("Wrote memory profile to %s.", profile.Name())
		if err := profile.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close file storing memory profile.")
		}
	}, interval)
}
