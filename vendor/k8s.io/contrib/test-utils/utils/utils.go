/*
Copyright 2015 The Kubernetes Authors.

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
	"strings"
	"sync"

	"github.com/golang/glog"
)

const (
	// KubekinsBucket is the name of the kubekins bucket
	KubekinsBucket = "kubernetes-jenkins"
	// LogDir is the directory of kubekins
	LogDir = "logs"
	// PullLogDir is the directory of the pr builder jenkins
	PullLogDir = "pr-logs"
	// PullKey is a string to look for in a job name to figure out if it's
	// a pull (presubmit) job.
	PullKey = "pull"

	// lookUpDirectory is the folder to look in to get output of the PR
	// builder run of a given number. This is needed because the PR builder
	// has to store its output in a location with the PR number in the
	// path.  So to find output, first we look here in the "directory" to
	// find the place we should actually read the output from.
	lookUpDirectory = "directory"

	successString = "SUCCESS"
)

// Utils is a struct handling all communication with a given bucket
type Utils struct {
	bucket        *Bucket
	directory     string
	pullKey       string
	pullDirectory string

	derefCache     map[string]string
	derefCacheLock sync.Mutex
}

// NewUtils returnes new Utils struct for a given bucket name and subdirectory
func NewUtils(bucket, directory string) *Utils {
	return &Utils{
		bucket:     NewBucket(bucket),
		directory:  directory,
		derefCache: map[string]string{},
	}
}

// NewWithPresubmitDetection returnes new Utils struct for a given bucket name
// and subdirectory. If a job name contains the presubmitKey, it will be gotten
// from the presubmitDirectory and trigger the dereferencing logic.
func NewWithPresubmitDetection(bucket, directory, presubmitKey, presubmitDirectory string) *Utils {
	return &Utils{
		bucket:        NewBucket(bucket),
		directory:     directory,
		pullKey:       presubmitKey,
		pullDirectory: presubmitDirectory,
		derefCache:    map[string]string{},
	}
}

// NewTestUtils returnes new Utils struct for a given url pointing to a file server.
func NewTestUtils(bucket, directory, url string) *Utils {
	return &Utils{
		bucket:     NewTestBucket(bucket, url),
		directory:  directory,
		derefCache: map[string]string{},
	}
}

// needsDeref returns true if we should use the alternate directory and do a
// dereferencing step to find where files are actually stored.
func (u *Utils) needsDeref(job string) bool {
	return u.pullKey != "" && strings.Contains(job, u.pullKey)
}

// deref reads the file in GCS to figure out where the desired file is.  We
// cache the path that is read, so this only adds an extra call to GCS the
// first time.
func (u *Utils) deref(job string, buildNumber int) (directory string, err error) {
	u.derefCacheLock.Lock()
	defer u.derefCacheLock.Unlock()
	cacheKey := fmt.Sprintf("%v/%v", job, buildNumber)
	if d, ok := u.derefCache[cacheKey]; ok {
		return d, nil
	}

	body, err := readResponse(u.bucket.ReadFile(
		u.pullDirectory,
		lookUpDirectory,
		job,
		fmt.Sprintf("%d.txt", buildNumber),
	))
	if err != nil {
		return "", err
	}
	gsPath := strings.TrimSpace(string(body))

	bucketPrefix := "gs://" + u.bucket.bucket
	if !strings.HasPrefix(gsPath, bucketPrefix) {
		return "", fmt.Errorf("unexpected format, did not start with %q: %v", bucketPrefix, gsPath)
	}
	out := strings.TrimPrefix(gsPath, bucketPrefix)
	u.derefCache[cacheKey] = out
	return out, nil
}

// helper function for extracting bytes from a response.
func readResponse(response *http.Response, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("request didn't succeed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got status code %v", response.StatusCode)
	}
	return ioutil.ReadAll(response.Body)
}

// GetPathToJenkinsGoogleBucket returns a GCS path containing the artifacts for
// a given job and buildNumber.  This only formats the path. It doesn't include
// a host or protocol necessary for a full URI.  Note: the purpose of this
// function was to allow us to change the host (to gubernator) without breaking
// detection of old links.
func (u *Utils) GetPathToJenkinsGoogleBucket(job string, buildNumber int) string {
	if u.needsDeref(job) {
		dir, err := u.deref(job, buildNumber)
		if err != nil {
			glog.Errorf("Unable to deref %v/%v: %v", job, buildNumber, err)
		} else {
			// TODO: make this function return an error instead of
			// falling through here.
			return u.bucket.ExpandPathURL(dir).Path + "/"
		}
	}
	return u.bucket.ExpandPathURL(u.directory, job, buildNumber).Path + "/"
}

// GetFileFromJenkinsGoogleBucket reads data from Google project's GCS bucket for the given job and buildNumber.
// Returns a response with file stored under a given (relative) path or an error.
func (u *Utils) GetFileFromJenkinsGoogleBucket(job string, buildNumber int, path string) (*http.Response, error) {
	if u.needsDeref(job) {
		dir, err := u.deref(job, buildNumber)
		if err != nil {
			return nil, fmt.Errorf("Couldn't deref %v/%v: %v", job, buildNumber, err)
		}
		return u.bucket.ReadFile(dir, path)
	}

	return u.bucket.ReadFile(u.directory, job, buildNumber, path)
}

// GetLastestBuildNumberFromJenkinsGoogleBucket reads a the number
// of last completed build of the given job from the Google project's GCS bucket.
func (u *Utils) GetLastestBuildNumberFromJenkinsGoogleBucket(job string) (int, error) {
	var response *http.Response
	var err error
	if u.needsDeref(job) {
		response, err = u.bucket.ReadFile(u.pullDirectory, lookUpDirectory, job, "latest-build.txt")
	} else {
		response, err = u.bucket.ReadFile(u.directory, job, "latest-build.txt")
	}
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
	if u.needsDeref(job) {
		dir, err := u.deref(job, buildNumber)
		if err != nil {
			return nil, fmt.Errorf("Couldn't deref %v/%v: %v", job, buildNumber, err)
		}
		return u.bucket.List(dir, prefix)
	}

	return u.bucket.List(u.directory, job, buildNumber, prefix)
}

// ListFilesWithPrefix returns all files with matching prefix in the bucket
// The returned file name included the complete path from bucket root
func (u *Utils) ListFilesWithPrefix(prefix string) ([]string, error) {
	return u.bucket.List(prefix)
}
