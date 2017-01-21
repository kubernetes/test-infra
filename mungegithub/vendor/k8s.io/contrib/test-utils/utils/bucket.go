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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const (
	// GCSListAPIHost is the host of the api for a bucket
	GCSListAPIHost = "www.googleapis.com"
	// GCSBucketHost is the host for GCS bucket directory requests
	GCSBucketHost = "storage.googleapis.com"

	retries   = 3
	retryWait = 100 * time.Millisecond
)

// Bucket represents a GCS bucket. It will craft file and list requests for you.
type Bucket struct {
	scheme     string
	bucket     string
	listHost   string
	bucketHost string
}

// NewBucket constructs a new bucket pointing at the default URLs.
func NewBucket(name string) *Bucket {
	return &Bucket{
		scheme:     "https",
		bucket:     name,
		listHost:   GCSListAPIHost,
		bucketHost: GCSBucketHost,
	}
}

// NewTestBucket constructs a new bucket pointing at a specific host.
func NewTestBucket(name string, host string) *Bucket {
	u, err := url.Parse(host)
	if err != nil {
		glog.Fatalf("test passed invalid host %v: %v", host, err)
	}
	return &Bucket{
		scheme:     u.Scheme,
		bucket:     name,
		listHost:   u.Host,
		bucketHost: u.Host,
	}
}

// ReadFile assembles the path and initiates a read operation.
func (b *Bucket) ReadFile(pathElements ...interface{}) (*http.Response, error) {
	url := b.ExpandPathURL(pathElements...)
	return getResponseWithRetry(url.String())
}

// ExpandPathURL turns the given path into a complete URL, good for accessing a
// file.
func (b *Bucket) ExpandPathURL(pathElements ...interface{}) *url.URL {
	// Prepend the bucket to the path
	pathElements = append([]interface{}{"/", b.bucket}, pathElements...)
	return &url.URL{
		Scheme: b.scheme,
		Host:   b.bucketHost,
		Path:   joinStringsAndInts(pathElements...),
	}
}

// ExpandListURL produces the URL for a list API query which will list files
// enclosed by the provided path. Note that there's currently no way to get a
// path that ends in '/'.
func (b *Bucket) ExpandListURL(pathElements ...interface{}) *url.URL {
	q := url.Values{}
	// GCS api doesn't like preceding '/', so remove it.
	q.Set("prefix", strings.TrimPrefix(joinStringsAndInts(pathElements...), "/"))
	return &url.URL{
		Scheme:   b.scheme,
		Host:     b.listHost,
		Path:     path.Join("storage/v1/b", b.bucket, "o"),
		RawQuery: q.Encode(),
	}
}

// List returns a list of all files inside the given path.
// The returned file name included the complete path from bucket root
func (b *Bucket) List(pathElements ...interface{}) ([]string, error) {
	listURL := b.ExpandListURL(pathElements...)
	res, err := getResponseWithRetry(listURL.String())
	if err != nil {
		return nil, fmt.Errorf("Failed to GET %v: %v", listURL, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Got a non-success response %v while listing %v", res.StatusCode, listURL.String())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read the response for %v: %v", listURL.String(), err)
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal %v: %v", string(body), err)
	}
	var ret []string
	if _, ok := data["items"]; !ok {
		glog.Warningf("No matching files were found (from: %v)", listURL.String())
		return ret, nil
	}
	for _, item := range data["items"].([]interface{}) {
		ret = append(ret, (item.(map[string]interface{})["name"]).(string))
	}
	return ret, nil
}

func joinStringsAndInts(pathElements ...interface{}) string {
	var parts []string
	for _, e := range pathElements {
		switch t := e.(type) {
		case string:
			parts = append(parts, t)
		case int:
			parts = append(parts, strconv.Itoa(t))
		default:
			glog.Fatalf("joinStringsAndInts only accepts ints and strings as path elements, but was passed %#v", t)
		}
	}
	return path.Join(parts...)
}

func getResponseWithRetry(url string) (*http.Response, error) {
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
