/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"flag"
	"time"
)

const (
	DefaultMetricsPort = 9090
	DefaultPProfPort   = 6060
	DefaultHealthPort  = 8081

	DefaultMemoryProfileInterval = 30 * time.Second
)

// InstrumentationOptions holds common options which are used across Prow components
type InstrumentationOptions struct {
	// MetricsPort is the port which is used to serve metrics
	MetricsPort int
	// PProfPort is the port which is used to serve pprof
	PProfPort int
	// HealthPort is the port which is used to serve liveness and readiness
	HealthPort int

	// ProfileMemory determines if the process should profile memory
	ProfileMemory bool
	// MemoryProfileInterval is the interval at which memory profiles should be dumped
	MemoryProfileInterval time.Duration
}

// AddFlags injects common options into the given FlagSet.
func (o *InstrumentationOptions) AddFlags(fs *flag.FlagSet) {
	fs.IntVar(&o.MetricsPort, "metrics-port", DefaultMetricsPort, "port to serve metrics")
	fs.IntVar(&o.PProfPort, "pprof-port", DefaultPProfPort, "port to serve pprof")
	fs.IntVar(&o.HealthPort, "health-port", DefaultHealthPort, "port to serve liveness and readiness")
	fs.BoolVar(&o.ProfileMemory, "profile-memory-usage", false, "profile memory usage for analysis")
	fs.DurationVar(&o.MemoryProfileInterval, "memory-profile-interval", DefaultMemoryProfileInterval, "duration at which memory profiles should be dumped")
}

func (o *InstrumentationOptions) Validate(_ bool) error {
	return nil
}
