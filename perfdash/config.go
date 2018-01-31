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
	"fmt"
	"github.com/ghodss/yaml"
	"io/ioutil"
	"k8s.io/contrib/test-utils/utils"
	"net/http"
	"os"
	"strings"
)

// To add new e2e test support, you need to:
//   1) Transform e2e performance test result into *PerfData* in k8s/kubernetes/test/e2e/perftype,
//   and print the PerfData in e2e test log.
//   2) Add corresponding bucket, job and test into *TestConfig*.

// TestDescription contains test name, output file prefix and parser function.
type TestDescription struct {
	Name             string
	OutputFilePrefix string
	Parser           func(data []byte, buildNumber int, testResult *BuildData)
}

// Tests is a map from test label to test description.
type Tests struct {
	Prefix       string
	Descriptions map[string]TestDescription
}

// Jobs is a map from job name to all supported tests in the job.
type Jobs map[string]Tests

// Buckets is a map from bucket url to all supported jobs in the bucket.
type Buckets map[string]Jobs

var (
	// performanceDescriptions contains metrics exported by a --ginko.focus=[Feature:Performance]
	// e2e test
	performanceDescriptions = map[string]TestDescription{
		"DensityResponsiveness": {
			Name:             "density",
			OutputFilePrefix: "APIResponsiveness",
			Parser:           parseResponsivenessData,
		},
		"DensityResources": {
			Name:             "density",
			OutputFilePrefix: "ResourceUsageSummary",
			Parser:           parseResourceUsageData,
		},
		"DensityPodStartup": {
			Name:             "density",
			OutputFilePrefix: "PodStartupLatency",
			Parser:           parseResponsivenessData,
		},
		"DensityTestPhaseTimer": {
			Name:             "density",
			OutputFilePrefix: "TestPhaseTimer",
			Parser:           parseResponsivenessData,
		},
		"LoadResponsiveness": {
			Name:             "load",
			OutputFilePrefix: "APIResponsiveness",
			Parser:           parseResponsivenessData,
		},
		"LoadResources": {
			Name:             "load",
			OutputFilePrefix: "ResourceUsageSummary",
			Parser:           parseResourceUsageData,
		},
		"LoadTestPhaseTimer": {
			Name:             "load",
			OutputFilePrefix: "TestPhaseTimer",
			Parser:           parseResponsivenessData,
		},
	}

	// TestConfig contains all the test PerfDash supports now. Downloader will download and
	// analyze build log from all these Jobs, and parse the data from all these Test.
	// Notice that all the tests should have different name for now.
	TestConfig = Buckets{utils.KubekinsBucket: getProwConfigOrDie()}
)

func getProwConfigOrDie() Jobs {
	jobs, err := getProwConfig()
	if err != nil {
		panic(err)
	}
	return jobs
}

// Minimal subset of the prow config definition at k8s.io/test-infra/prow/config
type config struct {
	Periodics []periodic `json:"periodics"`
}
type periodic struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func getProwConfig() (Jobs, error) {
	fmt.Fprintf(os.Stderr, "Fetching prow config from GitHub...\n")
	resp, err := http.Get("https://raw.githubusercontent.com/kubernetes/test-infra/master/prow/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("error fetching prow config from GitHub: %v", err)
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading prow config from GitHub: %v", err)
	}
	conf := &config{}
	if err := yaml.Unmarshal(b, conf); err != nil {
		return nil, fmt.Errorf("error unmarshaling prow config from GitHub: %v", err)
	}
	jobs := Jobs{}
	for _, periodic := range conf.Periodics {
		for _, tag := range periodic.Tags {
			if strings.HasPrefix(tag, "perfDashPrefix:") {
				split := strings.SplitN(tag, ":", 2)
				jobs[periodic.Name] = Tests{
					Prefix:       strings.TrimSpace(split[1]),
					Descriptions: performanceDescriptions,
				}
				break
			}
		}
	}
	fmt.Printf("Read config with %d jobs\n", len(jobs))
	return jobs, nil
}
