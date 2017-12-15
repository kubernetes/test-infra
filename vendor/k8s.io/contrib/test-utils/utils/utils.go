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
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/golang/glog"
)

const (
	// GCSListAPIURLTemplate is the template of GCS list api for a bucket
	GCSListAPIURLTemplate = "https://www.googleapis.com/storage/v1/b/%s/o"
	// GCSBucketURLTemplate is the tempalate for a GCS bucket directory
	GCSBucketURLTemplate = "https://storage.googleapis.com/%s/%s"
	// KubekinsBucket is the name of the kubekins bucket
	KubekinsBucket = "kubernetes-jenkins"
	// LogDir is the directory of kubekins
	LogDir = "logs"

	successString = "SUCCESS"
	retries       = 3
	retryWait     = 100 * time.Millisecond
)

// Utils is a struct handling all communication with a given bucket
type Utils struct {
	bucket      string
	directory   string
	overrideURL string
}

// NewUtils returnes new Utils struct for a given bucket name and subdirectory
func NewUtils(bucket, directory string) *Utils {
	return &Utils{bucket: bucket, directory: directory}
}

// NewTestUtils returnes new Utils struct for a given url pointing to a file server
func NewTestUtils(url string) *Utils {
	return &Utils{overrideURL: url}
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

// GetGCSDirectoryURL returns the url of the bucket directory
func (u *Utils) GetGCSDirectoryURL() string {
	// return overrideURL if specified
	if u.overrideURL != "" {
		return u.overrideURL
	}
	return fmt.Sprintf(GCSBucketURLTemplate, u.bucket, u.directory)
}

// GetGCSListURL returns the url to the list api
func (u *Utils) GetGCSListURL() string {
	// return overrideURL if specified
	if u.overrideURL != "" {
		return u.overrideURL
	}
	return fmt.Sprintf(GCSListAPIURLTemplate, u.bucket)
}

// GetPathToJenkinsGoogleBucket returns a GCS path containing the artifacts for a given job and buildNumber.
// This only formats the path. It doesn't include a host or protocol necessary for a full URI.
func (u *Utils) GetPathToJenkinsGoogleBucket(job string, buildNumber int) string {
	return fmt.Sprintf("/%s/%s/%s/%d/", u.bucket, u.directory, job, buildNumber)
}

// GetFileFromJenkinsGoogleBucket reads data from Google project's GCS bucket for the given job and buildNumber.
// Returns a response with file stored under a given (relative) path or an error.
func (u *Utils) GetFileFromJenkinsGoogleBucket(job string, buildNumber int, path string) (*http.Response, error) {
	response, err := u.getResponseWithRetry(fmt.Sprintf("%v/%v/%v/%v", u.GetGCSDirectoryURL(), job, buildNumber, path))
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetLastestBuildNumberFromJenkinsGoogleBucket reads a the number
// of last completed build of the given job from the Google project's GCS bucket .
func (u *Utils) GetLastestBuildNumberFromJenkinsGoogleBucket(job string) (int, error) {
	response, err := u.getResponseWithRetry(fmt.Sprintf("%v/%v/latest-build.txt", u.GetGCSDirectoryURL(), job))
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

// StartedFile is a type in which we store test starting informatio in GCS as started.json
type StartedFile struct {
	Version     string `json:"version"`
	Timestamp   uint64 `json:"timestamp"`
	JenkinsNode string `json:"jenkins-node"`
}

// CheckStartedStatus reads the started.json file for a given job and build number.
// It returns true if the result stored there is success, and false otherwise.
func (u *Utils) CheckStartedStatus(job string, buildNumber int) (*StartedFile, error) {
	response, err := u.GetFileFromJenkinsGoogleBucket(job, buildNumber, "started.json")
	if err != nil {
		glog.Errorf("Error while getting data for %v/%v/%v: %v", job, buildNumber, "started.json", err)
		return nil, err
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		glog.Errorf("Got a non-success response %v while reading data for %v/%v/%v", response.StatusCode, job, buildNumber, "started.json")
		return nil, err
	}
	result := &StartedFile{}
	err = json.NewDecoder(response.Body).Decode(result)
	if err != nil {
		glog.Errorf("Failed to read or unmarshal %v/%v/%v: %v", job, buildNumber, "started.json", err)
		return nil, err
	}
	return result, nil
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
		return false, fmt.Errorf("got status code %v", response.StatusCode)
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

// ListFilesInBuild takes build info and list all file names with matching prefix
// The returned file name included the complete path from bucket root
func (u *Utils) ListFilesInBuild(job string, buildNumber int, prefix string) ([]string, error) {
	combinePrefix := path.Join(u.directory, job, strconv.Itoa(buildNumber), prefix)
	ret, err := u.ListFilesWithPrefix(combinePrefix)
	return ret, err
}

// ListFilesWithPrefix returns all files with matching prefix in the bucket
// The returned file name included the complete path from bucket root
func (u *Utils) ListFilesWithPrefix(prefix string) ([]string, error) {
	listURL, _ := url.Parse(u.GetGCSListURL())
	q := listURL.Query()
	q.Set("prefix", prefix)
	listURL.RawQuery = q.Encode()
	res, err := u.getResponseWithRetry(listURL.String())
	if err != nil {
		glog.Errorf("Failed to GET %v: %v", listURL, err)
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		glog.Errorf("Got a non-success response %v while listing %v", res.StatusCode, listURL.String())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		glog.Errorf("Failed to read the response for %v: %v", listURL.String(), err)
		return nil, err
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		glog.Errorf("Failed to unmarshal %v: %v", string(body), err)
		return nil, err
	}
	var ret []string
	if _, ok := data["items"]; !ok {
		glog.Warningf("No matching files were found")
		return ret, nil
	}
	for _, item := range data["items"].([]interface{}) {
		ret = append(ret, (item.(map[string]interface{})["name"]).(string))
	}
	return ret, nil
}
