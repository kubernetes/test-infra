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
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

// Options exposes the configuration necessary
// for defining the process being watched and
// where in GCS an upload will land.
type Options struct {
	GcsOptions     *gcsupload.Options `json:"-"`
	WrapperOptions *wrapper.Options   `json:"-"`
}

// Validate ensures that the set of options are
// self-consistent and valid
func (o *Options) Validate() error {
	if err := o.WrapperOptions.Validate(); err != nil {
		return err
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

// BindOptions binds flags to options
func (o *Options) BindOptions(flags *flag.FlagSet) {
	gcsupload.BindOptions(o.GcsOptions, flags)
	wrapper.BindOptions(o.WrapperOptions, flags)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.GcsOptions.Complete(args)
}

// ResolveOptions will resolve the set of options, preferring
// to use the full JSON configuration variable but falling
// back to user-provided flags if the variable is not
// provided.
func ResolveOptions() (*Options, error) {
	options := &Options{}
	if jsonConfig, provided := os.LookupEnv(JSONConfigEnvVar); provided {
		if err := json.Unmarshal([]byte(jsonConfig), &options); err != nil {
			return options, fmt.Errorf("could not resolve config from env: %v", err)
		}
		return options, nil
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	gcsupload.BindOptions(options.GcsOptions, fs)
	wrapper.BindOptions(options.WrapperOptions, fs)
	fs.Parse(os.Args[1:])
	options.GcsOptions.Complete(fs.Args())

	return options, nil
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
