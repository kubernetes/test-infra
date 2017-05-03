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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"k8s.io/test-infra/boskos/common"
)

// Public Boskos client object
type Client struct {
	url       string
	resources []string
	owner     string
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
// Returns name of the resource on success.
func (c *Client) Acquire(rtype string, state string, dest string) (string, error) {
	r, err := c.acquire(rtype, state, dest)
	if err != nil {
		return "", err
	}

	c.resources = append(c.resources, r)
	return r, nil
}

// ReleaseAll returns all resource hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error {
	if len(c.resources) == 0 {
		return fmt.Errorf("No holding resource")
	}

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

// ReleaseOne returns one of owned resource back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name string, dest string) error {

	for idx, r := range c.resources {
		if r == name {
			c.resources[idx] = c.resources[len(c.resources)-1]
			c.resources = c.resources[:len(c.resources)-1]
			if err := c.release(r, dest); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("No resource name %v", name)
}

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error {
	if len(c.resources) == 0 {
		return fmt.Errorf("No holding resource")
	}

	for _, r := range c.resources {
		if err := c.update(r, state); err != nil {
			return err
		}
	}

	return nil
}

// UpdateOne signals update for one of the resource hold by the client.
func (c *Client) UpdateOne(name string, state string) error {
	for _, r := range c.resources {
		if r == name {
			if err := c.update(r, state); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("No resource name %v", name)
}

// Reset will scan all boskos resources of type, in state, last updated before expire, and set them to dest state.
// Returns a map of {resourceName:owner} for further actions.
func (c *Client) Reset(rtype string, state string, expire time.Duration, dest string) (map[string]string, error) {
	return c.reset(rtype, state, expire, dest)
}

// private methods

func (c *Client) popResource() (string, bool) {
	if len(c.resources) == 0 {
		return "", false
	}

	r := c.resources[len(c.resources)-1]
	c.resources = c.resources[:len(c.resources)-1]
	return r, true
}

func (c *Client) acquire(rtype string, state string, dest string) (string, error) {
	resp, err := http.Post(fmt.Sprintf("%v/acquire?type=%v&state=%v&dest=%v&owner=%v", c.url, rtype, state, dest, c.owner), "", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		var res = new(common.Resource)
		err = json.Unmarshal(body, &res)
		if err != nil {
			return "", err
		}
		return res.Name, nil
	}

	return "", fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
}

func (c *Client) release(name string, dest string) error {
	resp, err := http.Post(fmt.Sprintf("%v/release?name=%v&dest=%v&owner=%v", c.url, name, dest, c.owner), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func (c *Client) update(name string, state string) error {
	resp, err := http.Post(fmt.Sprintf("%v/update?name=%v&owner=%v&state=%v", c.url, name, c.owner, state), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func (c *Client) reset(rtype string, state string, expire time.Duration, dest string) (map[string]string, error) {
	rmap := make(map[string]string)
	resp, err := http.Post(fmt.Sprintf("%v/reset?type=%v&state=%v&expire=%v&dest=%v", c.url, rtype, state, expire.String(), dest), "", nil)
	if err != nil {
		return rmap, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return rmap, err
		}

		err = json.Unmarshal(body, &rmap)
		if err != nil {
			return rmap, err
		}
		return rmap, nil
	}

	return rmap, fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
}
