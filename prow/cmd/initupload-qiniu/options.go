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

	"k8s.io/test-infra/prow/gcsupload"
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
	return json.Unmarshal([]byte(config), o)
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

// Run will start the initupload job to upload the artifacts, logs and clone status.
func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	// use folder structure with gcs
	jobBasePath, _, _ := gcsupload.PathsForJob(o.GCSConfiguration, spec, "")

	uploadTargets := map[string]qiniu.UploadFunc{}

	var failed bool
	var mainRefSHA string
	if o.Log != "" {
		if failed, mainRefSHA, err = processCloneLog(o.Log, uploadTargets, jobBasePath); err != nil {
			return err
		}
	}

	started := specToStarted(spec, mainRefSHA)

	startedData, err := json.Marshal(&started)
	if err != nil {
		return fmt.Errorf("could not marshal starting data: %v", err)
	}
	key := jobBasePath + "/started.json"
	uploadTargets[key] = qiniu.DataUpload(key, bytes.NewReader(startedData))

	qn, err := qiniu.NewUploader(o.Bucket, o.AccessKey, o.SecretKey)
	if err != nil {
		return fmt.Errorf("failed to init qiniu uploader: %v", err)
	}

	if err := qn.Upload(uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to Qiniu: %v", err)
	}

	if failed {
		return errors.New("cloning the appropriate refs failed")
	}

	return nil
}
