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
	"bufio"
	"fmt"
	"os"

	"k8s.io/contrib/test-utils/utils"
)

// constants to use for downloading data.
const (
	logFile = "build-log.txt"
)

// GoogleGCSDownloader that gets data about Google results from the GCS repository
type GoogleGCSDownloader struct {
	Builds               int
	GoogleGCSBucketUtils *utils.Utils
}

// NewGoogleGCSDownloader creates a new GoogleGCSDownloader
func NewGoogleGCSDownloader(builds int) *GoogleGCSDownloader {
	return &GoogleGCSDownloader{
		Builds:               builds,
		GoogleGCSBucketUtils: utils.NewUtils(utils.KubekinsBucket, utils.LogDir),
	}
}

// TODO(random-liu): Only download and update new data each time.
func (g *GoogleGCSDownloader) getData() (TestToBuildData, error) {
	fmt.Print("Getting Data from GCS...\n")
	result := make(TestToBuildData)
	for job, tests := range TestConfig[utils.KubekinsBucket] {
		lastBuildNo, err := g.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		if err != nil {
			return result, err
		}
		fmt.Printf("Last build no: %v\n", lastBuildNo)
		for buildNumber := lastBuildNo; buildNumber > lastBuildNo-g.Builds && buildNumber > 0; buildNumber-- {
			fmt.Printf("Fetching build %v...\n", buildNumber)
			testDataResponse, err := g.GoogleGCSBucketUtils.GetFileFromJenkinsGoogleBucket(job, buildNumber, logFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error while fetching data: %v\n", err)
				continue
			}

			testDataBody := testDataResponse.Body
			defer testDataBody.Close()
			testDataScanner := bufio.NewScanner(testDataBody)
			parseTestOutput(testDataScanner, job, tests, buildNumber, result)
		}
	}
	return result, nil
}
