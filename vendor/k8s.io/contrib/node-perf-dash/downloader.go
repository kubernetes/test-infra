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
	"io"
	"log"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"k8s.io/contrib/test-utils/utils"
)

const (
	latestBuildFile = "latest-build.txt"
	testResultFile  = "build-log.txt"
	kubeletLogFile  = "kubelet.log"
)

var (
	// allTestData stores all parsed perf and time series data in memeory for each job.
	allTestData = map[string]*TestToBuildData{}
	// grabbedLastBuild stores the last build grabbed for each job.
	allGrabbedLastBuild = map[string]int{}
)

// Downloader is the interface that connects to a data source.
type Downloader interface {
	GetLastestBuildNumber(job string) (int, error)
	GetFile(job string, buildNumber int, logFilePath string) (io.ReadCloser, error)
}

// GetData fetch as much data as possible and result the result.
func GetData(job string, d Downloader) error {
	fmt.Printf("Getting Data from %s... (Job: %s)\n", *datasource, job)
	buildNr := *builds
	testData := *allTestData[job]
	grabbedLastBuild := allGrabbedLastBuild[job]

	lastBuildNo, err := d.GetLastestBuildNumber(job)
	if err != nil {
		return err
	}

	fmt.Printf("Last build no: %v (Job: %s)\n", lastBuildNo, job)

	endBuild := lastBuildNo
	startBuild := int(math.Max(math.Max(float64(lastBuildNo-buildNr), 0), float64(grabbedLastBuild))) + 1

	// Grab data from startBuild to endBuild.
	for buildNumber := startBuild; buildNumber <= endBuild; buildNumber++ {
		fmt.Printf("Fetching build %v... (Job: %s)\n", buildNumber, job)

		file, err := d.GetFile(job, buildNumber, testResultFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error while fetching data: %v (Job: %s)\n", err, job)
			return err
		}

		// testTime records the end time of each test, used to extract tracing events.
		testTime := TestTime{}
		parseTestOutput(bufio.NewScanner(file), job, buildNumber, testData, testTime)
		file.Close()

		if *tracing {
			// Grab and convert tracing data from Kubelet log into time series data format.
			tracingData := ParseKubeletLog(d, job, buildNumber, testTime)
			// Parse time series data.
			parseTracingData(bufio.NewScanner(strings.NewReader(tracingData)), job, buildNumber, testData)
		}
	}
	allGrabbedLastBuild[job] = lastBuildNo
	return nil
}

// LocalDownloader gets test data from local files.
type LocalDownloader struct {
}

// NewLocalDownloader creates a new LocalDownloader
func NewLocalDownloader() *LocalDownloader {
	return &LocalDownloader{}
}

// GetLastestBuildNumber returns the latest build number.
func (d *LocalDownloader) GetLastestBuildNumber(job string) (int, error) {
	file, err := os.Open(path.Join(*localDataDir, latestBuildFile))
	if err != nil {
		return -1, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()

	i, err := strconv.Atoi(scanner.Text())
	if err != nil {
		log.Fatal(err)
		return -1, err
	}
	return i, nil
}

// GetFile returns readcloser of the desired file.
func (d *LocalDownloader) GetFile(job string, buildNumber int, filePath string) (io.ReadCloser, error) {
	return os.Open(path.Join(*localDataDir, fmt.Sprintf("%d", buildNumber), filePath))
}

// GoogleGCSDownloader gets test data from Google Cloud Storage.
type GoogleGCSDownloader struct {
	GoogleGCSBucketUtils *utils.Utils
}

// NewGoogleGCSDownloader creates a new GoogleGCSDownloader
func NewGoogleGCSDownloader() *GoogleGCSDownloader {
	return &GoogleGCSDownloader{
		GoogleGCSBucketUtils: utils.NewUtils(utils.KubekinsBucket, utils.LogDir),
	}
}

// GetLastestBuildNumber returns the latest build number.
func (d *GoogleGCSDownloader) GetLastestBuildNumber(job string) (int, error) {
	// It returns -1 if the path is not found
	return d.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
}

// GetFile returns readcloser of the desired file.
func (d *GoogleGCSDownloader) GetFile(job string, buildNumber int, filePath string) (io.ReadCloser, error) {
	response, err := d.GoogleGCSBucketUtils.GetFileFromJenkinsGoogleBucket(job, buildNumber, filePath)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}
