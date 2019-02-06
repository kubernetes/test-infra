/*
Copyright 2019 The Kubernetes Authors.

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

package kube

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
)

// NewDryRunProwJobClient creates a new client that uses deck as a read-only proxy for ProwJob data
func NewDryRunProwJobClient(deckURL string) prowv1.ProwJobInterface {
	return &dryRunProwJobClient{
		deckURL: deckURL,
		client:  &http.Client{},
	}
}

// dryRunProwJobClient proxies through `deck` to provide a subset of the ProwJob client methods
type dryRunProwJobClient struct {
	deckURL string
	client  *http.Client
}

// Create does nothing on a dry-run client
func (c *dryRunProwJobClient) Create(*prowapi.ProwJob) (*prowapi.ProwJob, error) {
	return nil, nil
}

// Update does nothing on a dry-run client
func (c *dryRunProwJobClient) Update(*prowapi.ProwJob) (*prowapi.ProwJob, error) {
	return nil, nil
}

// UpdateStatus does nothing on a dry-run client
func (c *dryRunProwJobClient) UpdateStatus(*prowapi.ProwJob) (*prowapi.ProwJob, error) {
	return nil, nil
}

// Delete does nothing on a dry-run client
func (c *dryRunProwJobClient) Delete(name string, options *metav1.DeleteOptions) error {
	return nil
}

// DeleteCollection does nothing on a dry-run client
func (c *dryRunProwJobClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

// Get does nothing on a dry-run client
func (c *dryRunProwJobClient) Get(name string, options metav1.GetOptions) (*prowapi.ProwJob, error) {
	return nil, nil
}

// List reaches out to `deck` to retrieve the ProwJobs on the cluster via proxy
func (c *dryRunProwJobClient) List(opts metav1.ListOptions) (*prowapi.ProwJobList, error) {
	var jl prowapi.ProwJobList
	err := c.request("/prowjobs.js", map[string]string{"labelSelector": opts.LabelSelector}, &jl)
	return &jl, err
}

func (c *dryRunProwJobClient) request(path string, query map[string]string, ret interface{}) error {
	out, err := c.requestRetry(path, query)
	if err != nil {
		return err
	}
	if ret != nil {
		if err := json.Unmarshal(out, ret); err != nil {
			return err
		}
	}
	return nil
}

// Retry on transport failures. Does not retry on 500s.
func (c *dryRunProwJobClient) requestRetry(path string, query map[string]string) ([]byte, error) {
	resp, err := c.retry(path, query)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, &kapierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusNotFound,
			Reason: metav1.StatusReasonNotFound,
		}}
	} else if resp.StatusCode == 409 {
		return nil, &kapierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusConflict,
			Reason: metav1.StatusReasonAlreadyExists,
		}}
	} else if resp.StatusCode == 422 {
		return nil, &kapierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
		}}
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("response has status \"%s\" and body \"%s\"", resp.Status, string(rb))
	}
	return rb, nil
}

func (c *dryRunProwJobClient) retry(path string, query map[string]string) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := retryDelay
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(path, query)
		if err == nil {
			if resp.StatusCode < 500 {
				break
			}
			resp.Body.Close()
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return resp, err
}

func (c *dryRunProwJobClient) doRequest(urlPath string, query map[string]string) (*http.Response, error) {
	url := c.deckURL + urlPath
	req, err := http.NewRequest("", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	q := req.URL.Query()
	for k, v := range query {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return c.client.Do(req)
}

// Watch does nothing on a dry-run client
func (c *dryRunProwJobClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

// Patch does nothing on a dry-run client
func (c *dryRunProwJobClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *prowapi.ProwJob, err error) {
	return nil, nil
}

var _ prowv1.ProwJobInterface = &dryRunProwJobClient{}
