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

package initupload

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/prow/gcsupload"
)

const (
	// JSONConfigEnvVar is the environment variable that
	// utilities expect to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "INITUPLOAD_OPTIONS"
)

type Options struct {
	*gcsupload.Options

	// Log is the log file to which clone records are written
	Log string `json:"log"`
}

func (o *Options) Validate() error {
	if o.Log == "" {
		return errors.New("the path to the clone records log was not provided")
	}

	return o.Options.Validate()
}

// ResolveOptions will resolve the set of options,
// preferring to use the full JSON configuration variable but
// falling back to user-provided flags if the variable is not
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
	fs.StringVar(&options.Log, "log", "", "path to the output file for the cloning step")
	gcsupload.BindOptions(options.Options, fs)
	fs.Parse(os.Args[1:])
	options.Complete(fs.Args())

	return options, nil
}

// Encode will encode the set of options in the format
// that is expected for the configuration environment variable
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
