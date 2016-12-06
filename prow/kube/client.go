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

package kube

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

const (
	inClusterBaseURL = "https://kubernetes"
	maxRetries       = 8
	retryDelay       = 2 * time.Second
)

type Logger interface {
	Printf(s string, v ...interface{})
}

// Client interacts with the Kubernetes api-server.
type Client struct {
	// If Logger is non-nil, log all method calls with it.
	Logger Logger

	baseURL   string
	client    *http.Client
	token     string
	namespace string
	fake      bool
}

func (c *Client) log(methodName string, args ...interface{}) {
	if c.Logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	c.Logger.Printf("%s(%s)", methodName, strings.Join(as, ", "))
}

type ConflictError error

// Retry on transport failures. Does not retry on 500s.
func (c *Client) request(method, urlPath string, query map[string]string, body io.Reader) ([]byte, error) {
	if c.fake {
		return []byte("{}"), nil
	}
	var resp *http.Response
	var err error
	backoff := retryDelay
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(method, urlPath, query, body)
		if err == nil {
			break
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 409 {
		return nil, ConflictError(fmt.Errorf("body: %s", string(rb)))
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("response has status \"%s\" and body \"%s\"", resp.Status, string(rb))
	}
	return rb, nil
}

func (c *Client) doRequest(method, urlPath string, query map[string]string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + urlPath
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if method == http.MethodPatch {
		req.Header.Set("Content-Type", "application/strategic-merge-patch+json")
	} else {
		req.Header.Set("Content-Type", "application/json")
	}

	q := req.URL.Query()
	for k, v := range query {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return c.client.Do(req)
}

// NewFakeClient creates a client that doesn't do anything.
func NewFakeClient() *Client {
	return &Client{
		namespace: "default",
		fake:      true,
	}
}

// NewClientInCluster creates a Client that works from within a pod.
func NewClientInCluster(namespace string) (*Client, error) {
	tokenFile := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}

	rootCAFile := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	certData, err := ioutil.ReadFile(rootCAFile)
	if err != nil {
		return nil, err
	}

	cp := x509.NewCertPool()
	cp.AppendCertsFromPEM(certData)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    cp,
		},
	}
	c := &http.Client{Transport: tr}
	return &Client{
		baseURL:   inClusterBaseURL,
		client:    c,
		token:     string(token),
		namespace: namespace,
	}, nil
}

func (c *Client) GetPod(name string) (Pod, error) {
	c.log("GetPod", name)
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", c.namespace, name)
	body, err := c.request(http.MethodGet, path, map[string]string{}, nil)
	if err != nil {
		return Pod{}, err
	}
	var retPod Pod
	if err = json.Unmarshal(body, &retPod); err != nil {
		return Pod{}, err
	}
	return retPod, nil
}

func (c *Client) ListPods(labels map[string]string) ([]Pod, error) {
	c.log("ListPods", labels)
	var sel []string
	for k, v := range labels {
		sel = append(sel, fmt.Sprintf("%s = %s", k, v))
	}
	labelSelector := strings.Join(sel, ",")
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods", c.namespace)
	b, err := c.request(http.MethodGet, path, map[string]string{
		"labelSelector": labelSelector,
	}, nil)
	if err != nil {
		return nil, err
	}
	var pl struct {
		Items []Pod `json:"items"`
	}
	err = json.Unmarshal(b, &pl)
	if err != nil {
		return nil, err
	}
	return pl.Items, nil
}

func (c *Client) DeletePod(name string) error {
	c.log("DeletePod", name)
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", c.namespace, name)
	_, err := c.request(http.MethodDelete, path, map[string]string{}, nil)
	return err
}

func (c *Client) GetJob(name string) (Job, error) {
	c.log("GetJob", name)
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", c.namespace, name)
	body, err := c.request(http.MethodGet, path, map[string]string{}, nil)
	if err != nil {
		return Job{}, err
	}
	var retJob Job
	if err = json.Unmarshal(body, &retJob); err != nil {
		return Job{}, err
	}
	return retJob, nil
}

func (c *Client) ListJobs(labels map[string]string) ([]Job, error) {
	c.log("ListJobs", labels)
	var sel []string
	for k, v := range labels {
		sel = append(sel, fmt.Sprintf("%s = %s", k, v))
	}
	labelSelector := strings.Join(sel, ",")
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs", c.namespace)
	b, err := c.request(http.MethodGet, path, map[string]string{
		"labelSelector": labelSelector,
	}, nil)
	if err != nil {
		return nil, err
	}
	var jl struct {
		Items []Job `json:"items"`
	}
	err = json.Unmarshal(b, &jl)
	if err != nil {
		return nil, err
	}
	return jl.Items, nil
}

func (c *Client) CreatePod(p Pod) (Pod, error) {
	c.log("CreatePod", p)
	b, err := json.Marshal(p)
	if err != nil {
		return Pod{}, err
	}
	buf := bytes.NewBuffer(b)
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods", c.namespace)
	body, err := c.request(http.MethodPost, path, map[string]string{}, buf)
	if err != nil {
		return Pod{}, err
	}
	var retPod Pod
	if err = json.Unmarshal(body, &retPod); err != nil {
		return Pod{}, err
	}
	return retPod, nil
}

func (c *Client) CreateJob(j Job) (Job, error) {
	c.log("CreateJob", j)
	b, err := json.Marshal(j)
	if err != nil {
		return Job{}, err
	}
	buf := bytes.NewBuffer(b)
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs", c.namespace)
	body, err := c.request(http.MethodPost, path, map[string]string{}, buf)
	if err != nil {
		return Job{}, err
	}
	var retJob Job
	if err = json.Unmarshal(body, &retJob); err != nil {
		return Job{}, err
	}
	return retJob, nil
}

func (c *Client) DeleteJob(name string) error {
	c.log("DeleteJob", name)
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", c.namespace, name)
	_, err := c.request(http.MethodDelete, path, map[string]string{}, nil)
	return err
}

func (c *Client) PatchJob(name string, job Job) (Job, error) {
	c.log("PatchJob", name, job)
	b, err := json.Marshal(job)
	if err != nil {
		return Job{}, err
	}
	buf := bytes.NewBuffer(b)
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", c.namespace, name)
	body, err := c.request(http.MethodPatch, path, map[string]string{}, buf)
	if err != nil {
		return Job{}, err
	}
	var retJob Job
	if err = json.Unmarshal(body, &retJob); err != nil {
		return Job{}, err
	}
	return retJob, nil
}

func (c *Client) PatchJobStatus(name string, job Job) (Job, error) {
	c.log("PatchJobStatus", name, job)
	b, err := json.Marshal(job)
	if err != nil {
		return Job{}, err
	}
	buf := bytes.NewBuffer(b)
	path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s/status", c.namespace, name)
	body, err := c.request(http.MethodPatch, path, map[string]string{}, buf)
	if err != nil {
		return Job{}, err
	}
	var retJob Job
	if err = json.Unmarshal(body, &retJob); err != nil {
		return Job{}, err
	}
	return retJob, nil
}

func (c *Client) ReplaceSecret(name string, s Secret) error {
	// Ommission of the secret from the logs is purposeful.
	c.log("ReplaceSecret", name)
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(b)
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", c.namespace, name)
	_, err = c.request(http.MethodPut, path, nil, buf)
	return err
}
