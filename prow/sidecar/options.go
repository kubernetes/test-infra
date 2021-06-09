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

package sidecar

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

// NewOptions returns an empty Options with no nil fields
func NewOptions() *Options {
	return &Options{
		GcsOptions: gcsupload.NewOptions(),
		// Do not instantiate DeprecatedWrapperOptions by default
	}
}

// Options exposes the configuration necessary
// for defining the process being watched and
// where in GCS an upload will land.
type Options struct {
	GcsOptions               *gcsupload.Options `json:"gcs_options"`
	DeprecatedWrapperOptions *wrapper.Options   `json:"wrapper_options,omitempty"` // TODO(fejta): remove july 2019

	// Additional entries to wait for if set
	Entries []wrapper.Options `json:"entries,omitempty"`

	// EntryError requires all entries to pass in order to exit cleanly.
	EntryError bool `json:"entry_error,omitempty"`

	// IgnoreInterrupts instructs the waiting process to ignore interrupt
	// signals. An interrupt signal is sent to this process when the kubelet
	// decides to delete the test Pod. This may be as a result of:
	//  - the ProwJob exceeding the `default_job_timeout` as configured for Prow
	//  - the ProwJob exceeding the `timeout` as configured for the job itself
	//  - the Pod exceeding the `pod_running_timeout` as configured for Prow
	//  - cluster instability causing the Pod to be evicted
	//
	// When this happens, the `entrypoint` process also gets the signal, and
	// forwards it to the process under test. `entrypoint` will wait for the
	// test process to exit, either configured with:
	//  - `grace_period` in the default decoration configurations for Prow
	//  - `grace_period` in the job's specific configuration
	// After the grace period, `entrypoint` will forcefully terminate the test
	// process and signal to `sidecar` that the process has exited.
	//
	// In parallel, the kubelet will be waiting on the Pod's `terminationGracePeriod`
	// after sending the interrupt signal, at which point the kubelet will forcefully
	// terminate all containers in the Pod.
	//
	// If `ignore_interrupts` is set, `sidecar` will do nothing upon receipt of
	// the interrupt signal; this implicitly means that upload of logs and artifacts
	// will begin when the test process exits, which may be as long as the grace
	// period if the test process does not gracefully handle interrupts. This will
	// require that the user configures the Pod's termination grace period to be
	// longer than the `entrypoint` grace period for the test process and the time
	// taken by `sidecar` to upload all relevant artifacts.
	IgnoreInterrupts bool `json:"ignore_interrupts,omitempty"`

	// WriteMemoryProfile makes the program write a memory profile periodically while
	// it runs. Use the k8s.io/test-infra/hack/analyze-memory-profiles.py script to
	// load the data into time series and plot it for analysis.
	WriteMemoryProfile bool `json:"write_memory_profile,omitempty"`

	// CensoringOptions are options that pertain to censoring output before upload.
	CensoringOptions *CensoringOptions `json:"censoring_options,omitempty"`

	// SecretDirectories is deprecated, use censoring_options.secret_directories instead.
	SecretDirectories []string `json:"secret_directories,omitempty"`
	// CensoringConcurrency is deprecated, use censoring_options.censoring_concurrency instead.
	CensoringConcurrency *int64 `json:"censoring_concurrency,omitempty"`
	// CensoringBufferSize is deprecated, use censoring_options.censoring_buffer_size instead.
	CensoringBufferSize *int `json:"censoring_buffer_size,omitempty"`
}

type CensoringOptions struct {
	// SecretDirectories are paths to directories containing secret data. The contents
	// of these secret data files will be censored from the logs and artifacts uploaded
	// to the cloud.
	SecretDirectories []string `json:"secret_directories,omitempty"`
	// CensoringConcurrency is the maximum number of goroutines that should be censoring
	// artifacts and logs at any time. If unset, defaults to 10.
	CensoringConcurrency *int64 `json:"censoring_concurrency,omitempty"`
	// CensoringBufferSize is the size in bytes of the buffer allocated for every file
	// being censored. We want to keep as little of the file in memory as possible in
	// order for censoring to be reasonably performant in space. However, to guarantee
	// that we censor every instance of every secret, our buffer size must be at least
	// two times larger than the largest secret we are about to censor. While that size
	// is the smallest possible buffer we could use, if the secrets being censored are
	// small, censoring will not be performant as the number of I/O actions per file
	// would increase. If unset, defaults to 10MiB.
	CensoringBufferSize *int `json:"censoring_buffer_size,omitempty"`

	// IncludeDirectories are directories which should have their content censored, provided
	// as relative path globs from the base of the artifact directory for the test. If
	// present, only content in these directories will be censored. Entries in this list
	// are parsed with the go-zglob library, allowing for globbed matches.
	IncludeDirectories []string `json:"include_directories,omitempty"`

	// ExcludeDirectories are directories which should not have their content censored,
	// provided as relative path globs from the base of the artifact directory for the
	// test. If present, content in these directories will not be censored even if the
	// directory also matches a glob in IncludeDirectories. Entries in this list are
	// parsed with the go-zglob library, allowing for globbed matches.
	ExcludeDirectories []string `json:"exclude_directories,omitempty"`
}

func (o Options) entries() []wrapper.Options {
	var e []wrapper.Options
	if o.DeprecatedWrapperOptions != nil {
		e = append(e, *o.DeprecatedWrapperOptions)
	}
	return append(e, o.Entries...)
}

// Validate ensures that the set of options are
// self-consistent and valid
func (o *Options) Validate() error {
	opts := CensoringOptions{
		SecretDirectories:    o.SecretDirectories,
		CensoringConcurrency: o.CensoringConcurrency,
		CensoringBufferSize:  o.CensoringBufferSize,
	}
	if o.SecretDirectories != nil || o.CensoringConcurrency != nil || o.CensoringBufferSize != nil {
		if o.CensoringOptions != nil {
			return errors.New("cannot use deprecated options (secret_directories, censoring_{concurrency,buffer_size}) and new options (censoring_options) at the same time")
		}
		o.CensoringOptions = &opts
	}

	ents := o.entries()
	if len(ents) == 0 {
		return errors.New("no wrapper.Option entries")
	}
	for i, e := range ents {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("entry %d: %v", i, err)
		}
	}

	return o.GcsOptions.Validate()
}

const (
	// JSONConfigEnvVar is the environment variable that
	// utilities expect to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "SIDECAR_OPTIONS"
)

// ConfigVar exposese the environment variable used
// to store serialized configuration
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// AddFlags binds flags to options
func (o *Options) AddFlags(flags *flag.FlagSet) {
	o.GcsOptions.AddFlags(flags)
	// DeprecatedWrapperOptions flags should be unused, remove immediately
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.GcsOptions.Complete(args)
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
