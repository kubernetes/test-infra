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

package gcsupload

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
)

// NewOptions returns an empty Options with no nil fields.
func NewOptions() *Options {
	return &Options{
		GCSConfiguration: &prowapi.GCSConfiguration{},
	}
}

// Options exposes the configuration necessary
// for defining where in storage an upload will land.
type Options struct {
	// Items are files or directories to upload.
	Items []string `json:"items,omitempty"`

	// SubDir is appended to the GCS path.
	SubDir string `json:"sub_dir,omitempty"`

	*prowapi.GCSConfiguration

	prowflagutil.StorageClientOptions

	DryRun bool `json:"dry_run"`

	// mediaTypes holds additional extension media types to add to Go's
	// builtin's and the local system's defaults.  Values are
	// colon-delimited {extension}:{media-type}, for example:
	// log:text/plain.
	mediaTypes flagutil.Strings

	// gcsPath is used to store human-provided GCS
	// paths that are parsed to get more granular
	// fields.
	gcsPath gcs.Path
}

// Validate ensures that the set of options are
// self-consistent and valid.
func (o *Options) Validate() error {
	if o.LocalOutputDir != "" {
		return nil
	}
	if o.gcsPath.String() != "" {
		o.Bucket = o.gcsPath.Bucket()
		o.PathPrefix = o.gcsPath.Object()
	}

	if !o.DryRun {
		if o.Bucket == "" {
			return errors.New("GCS upload was requested no GCS bucket was provided")
		}
	}

	return o.GCSConfiguration.Validate()
}

// ConfigVar exposes the environment variable used
// to store serialized configuration.
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.Items = args

	for _, extensionMediaType := range o.mediaTypes.Strings() {
		parts := strings.SplitN(extensionMediaType, ":", 2)
		if len(parts) != 2 {
			panic(fmt.Sprintf("invalid extension media type %q: missing colon delimiter", extensionMediaType))
		}
		extension, mediaType := parts[0], parts[1]
		if o.GCSConfiguration.MediaTypes == nil {
			o.GCSConfiguration.MediaTypes = map[string]string{}
		}
		o.GCSConfiguration.MediaTypes[extension] = mediaType
	}
	o.mediaTypes = flagutil.NewStrings()
}

// AddFlags adds flags to the FlagSet that populate
// the GCS upload options struct given.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.SubDir, "sub-dir", "", "Optional sub-directory of the job's path to which artifacts are uploaded")

	fs.StringVar(&o.PathStrategy, "path-strategy", prowapi.PathStrategyExplicit, "how to encode org and repo into GCS paths")
	fs.StringVar(&o.DefaultOrg, "default-org", "", "optional default org for GCS path encoding")
	fs.StringVar(&o.DefaultRepo, "default-repo", "", "optional default repo for GCS path encoding")

	fs.Var(&o.gcsPath, "gcs-path", "GCS path to upload into")
	fs.BoolVar(&o.DryRun, "dry-run", true, "do not interact with GCS")

	fs.Var(&o.mediaTypes, "media-type", "Optional comma-delimited set of extension media types.  Each entry is colon-delimited {extension}:{media-type}, for example, log:text/plain.")

	fs.StringVar(&o.LocalOutputDir, "local-output-dir", "", "If specified, files are copied to this dir instead of uploading to GCS.")

	o.StorageClientOptions.AddFlags(fs)
}

const (
	// JSONConfigEnvVar is the environment variable that
	// utilities expect to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "GCSUPLOAD_OPTIONS"
)

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable.
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
