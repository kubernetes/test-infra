/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
)

const (
	// GoogleBucketURL is an url go GCS bucket with results of Google-run tests.
	GoogleBucketURL = "https://storage.googleapis.com/kubernetes-jenkins/logs"
	successString   = "SUCCESS"
	retries         = 3
	retryWait       = 100 * time.Millisecond
)

// Utils is a struct handling all communication with a given bucket
type Utils struct {
	bucketURL string
}

// NewUtils returnes new Utils struct for a given bucket url
func NewUtils(bucketURL string) *Utils {
	return &Utils{bucketURL: bucketURL}
}

func (u *Utils) getResponseWithRetry(url string) (*http.Response, error) {
	var response *http.Response
	var err error
	for i := 0; i < retries; i++ {
		response, err = http.Get(url)
		if err != nil {
			return nil, err
		}
		if response.StatusCode == http.StatusOK {
			return response, nil
		}
		time.Sleep(retryWait)
	}
	return response, nil
}

// GetFileFromJenkinsGoogleBucket reads data from Google project's GCS bucket for the given job and buildNumber.
// Returns a response with file stored under a given (relative) path or an error.
func (u *Utils) GetFileFromJenkinsGoogleBucket(job string, buildNumber int, path string) (*http.Response, error) {
	response, err := u.getResponseWithRetry(fmt.Sprintf("%v/%v/%v/%v", u.bucketURL, job, buildNumber, path))
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetLastestBuildNumberFromJenkinsGoogleBucket reads a the number
// of last completed build of the given job from the Google project's GCS bucket .
func (u *Utils) GetLastestBuildNumberFromJenkinsGoogleBucket(job string) (int, error) {
	response, err := u.getResponseWithRetry(fmt.Sprintf("%v/%v/latest-build.txt", u.bucketURL, job))
	if err != nil {
		return -1, err
	}
	body := response.Body
	defer body.Close()
	if response.StatusCode != http.StatusOK {
		glog.Infof("Got a non-success response %v while reading data for %v/latest-build.txt", response.StatusCode, job)
		return -1, err
	}
	scanner := bufio.NewScanner(body)
	scanner.Scan()
	var lastBuildNo int
	fmt.Sscanf(scanner.Text(), "%d", &lastBuildNo)
	return lastBuildNo, nil
}

// FinishedFile is a type in which we store test result in GCS as finished.json
type FinishedFile struct {
	Result    string `json:"result"`
	Timestamp uint64 `json:"timestamp"`
}

// CheckFinishedStatus reads the finished.json file for a given job and build number.
// It returns true if the result stored there is success, and false otherwise.
func (u *Utils) CheckFinishedStatus(job string, buildNumber int) (bool, error) {
	response, err := u.GetFileFromJenkinsGoogleBucket(job, buildNumber, "finished.json")
	if err != nil {
		glog.Errorf("Error while getting data for %v/%v/%v: %v", job, buildNumber, "finished.json", err)
		return false, err
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		glog.Errorf("Got a non-success response %v while reading data for %v/%v/%v", response.StatusCode, job, buildNumber, "finished.json")
		return false, err
	}
	result := FinishedFile{}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, buildNumber, "finished.json", err)
		return false, err
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		glog.Errorf("Failed to unmarshal %v: %v", string(body), err)
		return false, err
	}
	return result.Result == successString, nil
}
