/*
Copyright 2017 The Kubernetes Authors.

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

package mungeopts

import (
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/options"
)

var (
	// GCS holds the values of GCS options.
	GCS struct {
		BucketName string
		LogDir     string

		// PullLogDir is the directory of the pr builder jenkins
		PullLogDir string

		// PullKey is a string to look for in a job name to figure out if it's
		// a pull (presubmit) job.
		PullKey string
	}

	// RequiredContexts holds options that specify which status contexts are required for various
	// actions.
	RequiredContexts struct {
		Merge  []string
		Retest []string
	}

	// Maximum time to wait for tests in a PR to start or finish.
	// This should be >2x as long as it normally takes for a PR
	// to complete, to avoid congestion collapse in the queue.
	PRMaxWaitTime time.Duration

	// App is the mungegithub app (like 'submit-queue', 'cherrypick', or 'misc-mungers')
	App string
)

// RegisterOptions registers options that may be used by any munger, feature, or report. It returns
// any options that require a restart when changed.
func RegisterOptions(opts *options.Options) sets.String {
	// GCS options:
	opts.RegisterString(&GCS.BucketName, "gcs-bucket", "", "Name of GCS bucket.")
	opts.RegisterString(&GCS.LogDir, "gcs-logs-dir", "", "Directory containing test logs.")
	opts.RegisterString(&GCS.PullLogDir, "pull-logs-dir", "", "Directory of the PR builder.")
	opts.RegisterString(&GCS.PullKey, "pull-key", "", "String to look for in job name for it to be a pull (presubmit) job.")

	// Status context options:
	opts.RegisterStringSlice(&RequiredContexts.Retest, "required-retest-contexts", []string{}, "Comma separate list of statuses which will be retested and which must come back green after the `retest-body` comment is posted to a PR")
	opts.RegisterStringSlice(&RequiredContexts.Merge, "required-contexts", []string{}, "Comma separate list of status contexts required for a PR to be considered ok to merge")

	opts.RegisterDuration(&PRMaxWaitTime, "pr-max-wait-time", 2*time.Hour, "Maximum time to wait for tests in a PR to start or finish")

	opts.RegisterString(&App, "app", "submit-queue", "The mungegithub app that this instance represents.")

	return sets.NewString("app")
}
