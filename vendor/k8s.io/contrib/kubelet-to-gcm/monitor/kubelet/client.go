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

package kubelet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/stats"
)

// Client contains all the information and methods to encapsulate
// communication with the Kubelet.
type Client struct {
	client     *http.Client
	summaryURL *url.URL
}

// NewClient returns a new Client.
func NewClient(host string, port uint, client *http.Client) (*Client, error) {
	// Parse our URL upfront, so we can fail fast.
	urlStr := fmt.Sprintf("http://%s:%d/stats/summary", host, port)
	summaryURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return &Client{
		client:     client,
		summaryURL: summaryURL,
	}, nil
}

// doRequestAndUnmarshal makes the request, and unmarshals the response into value.
func (k *Client) doRequestAndUnmarshal(client *http.Client, req *http.Request, value interface{}) error {
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body - %v", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%q not found", req.URL.String())
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed - %q, response: %q", response.Status, string(body))
	}
	err = json.Unmarshal(body, value)
	if err != nil {
		return fmt.Errorf("failed to parse output. Response: %q. Error: %v", string(body), err)
	}
	return nil
}

// GetSummary gets the kubelet's Summary metrics.
func (k *Client) GetSummary() (*stats.Summary, error) {
	req, err := http.NewRequest("GET", k.summaryURL.String(), nil)
	if err != nil {
		return nil, err
	}
	summary := &stats.Summary{}
	client := k.client
	if client == nil {
		client = http.DefaultClient
	}
	err = k.doRequestAndUnmarshal(client, req, summary)
	return summary, err
}
