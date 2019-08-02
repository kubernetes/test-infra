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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/qiniu"
)

const (
	// JSONConfigEnvVar is the environment variable where utilities expect to find a full JSON
	// configuration.
	JSONConfigEnvVar = "INITUPLOAD_OPTIONS"
)

// NewOptions returns an empty Options with no nil fields.
func NewOptions() *Options {
	return &Options{
		Options: qiniu.NewOptions(),
	}
}

// Options holds the GCS options and the log file of clone records.
type Options struct {
	*qiniu.Options

	// Log is the log file to which clone records are written. If unspecified, no clone records
	// are uploaded.
	Log string `json:"log,omitempty"`
}

// ConfigVar exposes the environment variable used to store serialized configuration.
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config.
func (o *Options) LoadConfig(config string) error {
	if err := json.Unmarshal([]byte(config), o); err != nil {
		return err
	}

	// TODO(CarlJi):理论上这些配置应该放到CRD里，这样全局就可以传递
	// 但考虑到操作CRD，风险较高，这里为了简化，希望使用者外部直接传入这些信息，或者使用默认环境变量的值
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	o.AddFlags(fs)
	return fs.Parse(os.Args[1:])
}

// AddFlags binds flags to options.
func (o *Options) AddFlags(flags *flag.FlagSet) {
	flags.StringVar(&o.Log, "clone-log", "", "Path to the clone records log")
	o.Options.AddFlags(flags)
}

// Complete internalizes command line arguments.
func (o *Options) Complete(args []string) {
	o.Options.Complete(args)
}

// Encode will encode the set of options in the format that is expected for the configuration
// environment variable.
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}

// Start will start the initupload job to upload the artifacts, logs and clone status.
func (o Options) Start() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	uploadTargets := map[string]qiniu.UploadFunc{}

	var failed bool
	var mainRefSHA string
	if o.Log != "" {
		if failed, mainRefSHA, err = processCloneLog(o.Log, uploadTargets); err != nil {
			return err
		}
	}

	started := specToStarted(spec, mainRefSHA)

	startedData, err := json.Marshal(&started)
	if err != nil {
		return fmt.Errorf("could not marshal starting data: %v", err)
	}

	uploadTargets["started.json"] = qiniu.DataUpload(bytes.NewReader(startedData))

	if err := o.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to QINIU: %v", err)
	}

	if failed {
		return errors.New("cloning the appropriate refs failed")
	}

	return nil
}
