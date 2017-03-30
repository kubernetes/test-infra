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

	boskos "k8s.io/test-infra/boskos/common"
)

var (
	defaultURL = "http://boskos"
)

type Client struct {
	url       string
	resources []string
	owner     string
}

func (c *Client) popResource() (string, bool) {
	if len(c.resources) == 0 {
		return "", false
	}

	r := c.resources[len(c.resources)-1]
	c.resources = c.resources[:len(c.resources)-1]
	return r, true
}

func (c *Client) Start(rtype string, state string) (string, error) {
	r, err := c.start(rtype, state)
	if err != nil {
		return "", err
	}

	c.resources = append(c.resources, r)
	return r, nil
}

func (c *Client) DoneAll(state string) error {
	if len(c.resources) == 0 {
		return fmt.Errorf("No holding resource")
	}

	for {
		r, ok := c.popResource()
		if !ok {
			break
		}

		if err := c.done(r, state); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) DoneOne(name string, state string) error {

	for idx, r := range c.resources {
		if r == name {
			c.resources[idx] = c.resources[len(c.resources)-1]
			c.resources = c.resources[:len(c.resources)-1]
			if err := c.done(r, state); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("No resource name %v", name)
}

func (c *Client) UpdateAll() error {
	if len(c.resources) == 0 {
		return fmt.Errorf("No holding resource")
	}

	for _, r := range c.resources {
		if err := c.update(r); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) UpdateOne(name string) error {
	for _, r := range c.resources {
		if r == name {
			if err := c.update(r); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("No resource name %v", name)
}

func (c *Client) Reset(rtype string, state string, expire time.Duration, dest string) (map[string]string, error) {
	return c.reset(rtype, state, expire, dest)
}

func NewClient(url string, owner string) *Client {
	if url == "" {
		url = defaultURL
	}

	client := &Client{
		url:   url,
		owner: owner,
	}

	return client
}

func (c *Client) start(rtype string, state string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("%v/start?type=%v&state=%v&owner=%v", c.url, rtype, state, c.owner))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		var res = new(boskos.Resource)
		err = json.Unmarshal(body, &res)
		if err != nil {
			return "", err
		}
		return res.Name, nil
	}

	return "", fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
}

func (c *Client) done(name string, state string) error {
	resp, err := http.Get(fmt.Sprintf("%v/done?name=%v&state=%v", c.url, name, state))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func (c *Client) update(name string) error {
	resp, err := http.Get(fmt.Sprintf("%v/update?name=%v", c.url, name))
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
	resp, err := http.Get(fmt.Sprintf("%v/reset?type=%v&state=%v&expire=%v&dest=%v", c.url, rtype, state, expire.String(), dest))
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
