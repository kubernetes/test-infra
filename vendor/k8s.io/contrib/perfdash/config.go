/*
Copyright 2016 The Kubernetes Authors.

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
	"k8s.io/contrib/test-utils/utils"
)

// To add new e2e test support, you need to:
//   1) Transform e2e performance test result into *PerfData* in k8s/kubernetes/test/e2e/perftype,
//   and print the PerfData in e2e test log.
//   2) Add corresponding bucket, job and test into *TestConfig*.

// Tests is a map from test description to test name.
type Tests map[string]string

// Jobs is a map from job name to all supported tests in the job.
type Jobs map[string]Tests

// Buckets is a map from bucket url to all supported jobs in the bucket.
type Buckets map[string]Jobs

var (
	// TestConfig contains all the test PerfDash supports now. Downloader will download and
	// analyze build log from all these Jobs, and parse the data from all these Test.
	// Notice that all the tests should have different name for now.
	TestConfig = Buckets{
		utils.KubekinsBucket: Jobs{
			"kubernetes-e2e-gce-scalability": Tests{
				"[Feature:Performance] should allow starting 30 pods per node":    "Density",
				"[Feature:Performance] should be able to handle 30 pods per node": "Load",
			},
			"kubernetes-e2e-gce-serial": Tests{
				"resource tracking for 35 pods per node":  "Kubelet Perf 35",
				"resource tracking for 100 pods per node": "Kubelet Perf 100",
			},
		},
	}

	// TestNameSeparator is the prefix of time name.
	TestNameSeparator = "[It] "
)
