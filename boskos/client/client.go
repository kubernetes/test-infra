/*
Copyright 2017 The Kubernetes Authors.

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

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"k8s.io/test-infra/boskos/common"
	"strings"
)

// Client defines the public Boskos client object
type Client struct {
	owner string
	url   string

	lock      sync.Mutex
	resources []string
}

// NewClient creates a boskos client, with boskos url and owner of the client.
func NewClient(owner string, url string) *Client {

	client := &Client{
		url:   url,
		owner: owner,
	}

	return client
}

// public method

// Acquire asks boskos for a resource of certain type in certain state, and set the resource to dest state.
// Returns the resource on success.
func (c *Client) Acquire(rtype, state, dest string) (*common.Resource, error) {
	r, err := c.acquire(rtype, state, dest)
	if err != nil {
		return nil, err
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	if r != nil {
		c.resources = append(c.resources, r.Name)
	}

	return r, nil
}

// AcquireByState asks boskos for a resources of certain type, and set the resource to dest state.
// Returns a list of resources on success.
func (c *Client) AcquireByState(state, dest string, names []string) ([]common.Resource, error) {
	resources, err := c.acquireByState(state, dest, names)
	if err != nil {
		return nil, err
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, resource := range resources {
		c.resources = append(c.resources, resource.Name)
	}
	return resources, nil
}

// ReleaseAll returns all resources hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error {
	c.lock.Lock()

	if len(c.resources) == 0 {
		c.lock.Unlock()
		return fmt.Errorf("no holding resource")
	}
	c.lock.Unlock()

	for {
		r, ok := c.popResource()
		if !ok {
			break
		}

		if err := c.release(r, dest); err != nil {
			return err
		}
	}
	return nil
}

// ReleaseOne returns one of owned resources back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name, dest string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	for idx, r := range c.resources {
		if r == name {
			c.resources[idx] = c.resources[len(c.resources)-1]
			c.resources = c.resources[:len(c.resources)-1]
			err := c.release(r, dest)
			return err
		}
	}

	return fmt.Errorf("no resource name %v", name)
}

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if len(c.resources) == 0 {
		return fmt.Errorf("no holding resource")
	}

	for _, r := range c.resources {
		if err := c.update(r, state, nil); err != nil {
			return err
		}
	}

	return nil
}

// UpdateOne signals update for one of the resources hold by the client.
func (c *Client) UpdateOne(name, state string, userData common.UserData) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, r := range c.resources {
		if r == name {
			err := c.update(r, state, userData)
			return err
		}
	}

	return fmt.Errorf("no resource name %v", name)
}

// Reset will scan all boskos resources of type, in state, last updated before expire, and set them to dest state.
// Returns a map of {resourceName:owner} for further actions.
func (c *Client) Reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	return c.reset(rtype, state, expire, dest)
}

// Metric will query current metric for target resource type.
// Return a common.Metric object on success.
func (c *Client) Metric(rtype string) (common.Metric, error) {
	return c.metric(rtype)
}

// HasResource tells if current client holds any resources
func (c *Client) HasResource() bool {
	return len(c.resources) > 0
}

// private methods

func (c *Client) popResource() (string, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if len(c.resources) == 0 {
		return "", false
	}

	r := c.resources[len(c.resources)-1]
	c.resources = c.resources[:len(c.resources)-1]
	return r, true
}

func (c *Client) acquire(rtype, state, dest string) (*common.Resource, error) {
	resp, err := http.Post(fmt.Sprintf("%v/acquire?type=%v&state=%v&dest=%v&owner=%v",
		c.url, rtype, state, dest, c.owner), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		res := common.Resource{}
		err = json.Unmarshal(body, &res)
		if err != nil {
			return nil, err
		}
		if res.Name == "" {
			return nil, fmt.Errorf("unable to parse resource")
		}
		return &res, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("resource not found")
	}
	return nil, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
}

func (c *Client) acquireByState(state, dest string, names []string) ([]common.Resource, error) {
	resp, err := http.Post(fmt.Sprintf("%v/acquirebystate?state=%v&dest=%v&owner=%v&names=%v",
		c.url, state, dest, c.owner, strings.Join(names, ",")), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var resources []common.Resource
		if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
			return nil, err
		}
		return resources, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("resources already used by another user")

	case http.StatusNotFound:
		return nil, fmt.Errorf("resources not found")
	}
	return nil, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
}

func (c *Client) release(name, dest string) error {
	resp, err := http.Post(fmt.Sprintf("%v/release?name=%v&dest=%v&owner=%v",
		c.url, name, dest, c.owner), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s, statusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func (c *Client) update(name, state string, userData common.UserData) error {
	var body io.Reader
	if userData != nil {
		b := new(bytes.Buffer)
		err := json.NewEncoder(b).Encode(userData)
		if err != nil {
			return err
		}
		body = b
	}
	resp, err := http.Post(fmt.Sprintf("%v/update?name=%v&owner=%v&state=%v",
		c.url, name, c.owner, state), "application/json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func (c *Client) reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	rmap := make(map[string]string)
	resp, err := http.Post(fmt.Sprintf("%v/reset?type=%v&state=%v&expire=%v&dest=%v",
		c.url, rtype, state, expire.String(), dest), "", nil)
	if err != nil {
		return rmap, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return rmap, err
		}

		err = json.Unmarshal(body, &rmap)
		return rmap, err
	}

	return rmap, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
}

func (c *Client) metric(rtype string) (common.Metric, error) {
	var metric common.Metric
	resp, err := http.Get(fmt.Sprintf("%v/metric?type=%v", c.url, rtype))
	if err != nil {
		return metric, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return metric, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return metric, err
	}

	err = json.Unmarshal(body, &metric)
	return metric, err
}
