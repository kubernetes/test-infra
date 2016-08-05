/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package features

import (
	"k8s.io/contrib/mungegithub/github"

	"github.com/spf13/cobra"
)

const (
	// GCSFeature is how mungers should indicate this is required.
	GCSFeature = "google-cloud-storage"
)

// GCSInfo is a struct that handles parameters required by GCS to
// read log files and determine the status of tests.
type GCSInfo struct {
	BucketName string
	LogDir     string

	// PullLogDir is the directory of the pr builder jenkins
	PullLogDir string

	// PullKey is a string to look for in a job name to figure out if it's
	// a pull (presubmit) job.
	PullKey string
}

func init() {
	RegisterFeature(&GCSInfo{})
}

// Name is just going to return the name mungers use to request this feature
func (g *GCSInfo) Name() string {
	return GCSFeature
}

// Initialize will initialize the feature.
func (g *GCSInfo) Initialize(config *github.Config) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (g *GCSInfo) EachLoop() error {
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (g *GCSInfo) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&g.BucketName, "gcs-bucket", "", "Name of GCS bucket.")
	cmd.Flags().StringVar(&g.LogDir, "gcs-logs-dir", "", "Directory containing test logs.")
	cmd.Flags().StringVar(&g.PullLogDir, "pull-logs-dir", "", "Directory of the PR builder.")
	cmd.Flags().StringVar(&g.PullKey, "pull-key", "", "String to look for in job name for it to be a pull (presubmit) job.")
}
