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

package controller

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Metrics are parsed values from the kube-controller.
type Metrics struct {
	CreateTime    int64
	NodeEvictions int64
}

// parseMetrics takes the text format for prometheus metrics, and converts
// them into our Metrics object.
func (c *Metrics) parseMetrics(data string) error {
	dec := expfmt.NewDecoder(strings.NewReader(data), expfmt.FmtText)
	decoder := expfmt.SampleDecoder{
		Dec:  dec,
		Opts: &expfmt.DecodeOptions{},
	}

	for {
		var v model.Vector
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				// Expected loop termination condition.
				return nil
			}
			return fmt.Errorf("Invalid decode: %v", err)
		}
		for _, metric := range v {
			switch name := string(metric.Metric[model.MetricNameLabel]); name {
			case "node_collector_evictions_number":
				c.NodeEvictions = int64(metric.Value)
			case "process_start_time_seconds":
				c.CreateTime = int64(metric.Value)
			}
		}
	}
}

// NewMetrics creates a Metrics object from a Prometheus response body.
func NewMetrics(body []byte) (*Metrics, error) {
	metrics := &Metrics{}
	if err := metrics.parseMetrics(string(body)); err != nil {
		return nil, fmt.Errorf("Failed to create a new Metrics object: %v", err)
	}
	return metrics, nil
}

// Client queries metrics from the controller process.
type Client struct {
	client     *http.Client
	metricsURL *url.URL
}

// NewClient generates a client to hit the given controller.
func NewClient(host string, port uint, client *http.Client) (*Client, error) {
	// Parse our URL upfront, so we can fail fast.
	urlStr := fmt.Sprintf("http://%s:%d/metrics", host, port)
	metricsURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return &Client{
		client:     client,
		metricsURL: metricsURL,
	}, nil
}

// doRequest makes the request to the controller, and returns the body.
func (c *Client) doRequestAndParse(req *http.Request) (*Metrics, error) {
	response, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body - %v", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%q not found", req.URL.String())
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed - %q, response: %q", response.Status, string(body))
	}

	metrics, err := NewMetrics(body)
	return metrics, err
}

// GetMetrics returns the latest Metrics parsed from the kube-controller endpoint.
func (c *Client) GetMetrics() (*Metrics, error) {
	req, err := http.NewRequest("GET", c.metricsURL.String(), nil)
	if err != nil {
		return nil, err
	}
	metrics, err := c.doRequestAndParse(req)
	return metrics, err
}
