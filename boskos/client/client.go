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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/storage"
)

var (
	// ErrNotFound is returned by Acquire() when no resources are available.
	ErrNotFound = errors.New("resources not found")
	// ErrAlreadyInUse is returned by Acquire when resources are already being requested.
	ErrAlreadyInUse = errors.New("resources already used by another user")
)

// Client defines the public Boskos client object
type Client struct {
	// RetryCount is the number of times an HTTP request issued by this client
	// is retried when the initial request fails due an inaccessible endpoint.
	RetryCount uint

	// RetryDuration is the interval to wait before retrying an HTTP operation
	// that failed due to an inaccessible endpoint.
	RetryWait time.Duration

	owner string
	url   string
	lock  sync.Mutex

	storage storage.PersistenceLayer
}

// NewClient creates a boskos client, with boskos url, owner of the client,
// a RetryCount of 3, and a RetryWait interval of 10s.
func NewClient(owner string, url string) *Client {

	client := &Client{
		RetryCount: 3,
		RetryWait:  10 * time.Second,
		url:        url,
		owner:      owner,
		storage:    storage.NewMemoryStorage(),
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
		c.storage.Add(*r)
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
	for _, r := range resources {
		c.storage.Add(r)
	}
	return resources, nil
}

// ReleaseAll returns all resources hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return fmt.Errorf("no holding resource")
	}
	var allErrors error
	for _, r := range resources {
		c.storage.Delete(r.GetName())
		err := c.release(r.GetName(), dest)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// ReleaseOne returns one of owned resources back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name, dest string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, err := c.storage.Get(name); err != nil {
		return fmt.Errorf("no resource name %v", name)
	}
	c.storage.Delete(name)
	if err := c.release(name, dest); err != nil {
		return err
	}
	return nil
}

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return fmt.Errorf("no holding resource")
	}
	var allErrors error
	for _, r := range resources {
		if err := c.update(r.GetName(), state, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
		if err := c.updateLocalResource(r, state, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// SyncAll signals update for all resources hold by the client.
func (c *Client) SyncAll() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		logrus.Info("no resource to sync")
		return nil
	}
	var allErrors error
	for _, i := range resources {
		r, err := common.ItemToResource(i)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
		if err := c.update(r.Name, r.State, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
		if err := c.storage.Update(r); err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// UpdateOne signals update for one of the resources hold by the client.
func (c *Client) UpdateOne(name, state string, userData *common.UserData) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	r, err := c.storage.Get(name)
	if err != nil {
		return fmt.Errorf("no resource name %v", name)
	}
	if err := c.update(r.GetName(), state, userData); err != nil {
		return err
	}
	return c.updateLocalResource(r, state, userData)
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
	resources, _ := c.storage.List()
	return len(resources) > 0
}

// private methods

func (c *Client) updateLocalResource(i common.Item, state string, data *common.UserData) error {
	res, err := common.ItemToResource(i)
	if err != nil {
		return err
	}
	res.State = state
	if res.UserData == nil {
		res.UserData = data
	} else {
		res.UserData.Update(data)
	}

	return c.storage.Update(res)
}

func (c *Client) acquire(rtype, state, dest string) (*common.Resource, error) {
	resp, err := c.httpPost(fmt.Sprintf("%v/acquire?type=%v&state=%v&dest=%v&owner=%v",
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
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
}

func (c *Client) acquireByState(state, dest string, names []string) ([]common.Resource, error) {
	resp, err := c.httpPost(fmt.Sprintf("%v/acquirebystate?state=%v&dest=%v&owner=%v&names=%v",
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
		return nil, ErrAlreadyInUse
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode)
}

func (c *Client) release(name, dest string) error {
	resp, err := c.httpPost(fmt.Sprintf("%v/release?name=%v&dest=%v&owner=%v",
		c.url, name, dest, c.owner), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s, statusCode %v releasing %s", resp.Status, resp.StatusCode, name)
	}
	return nil
}

func (c *Client) update(name, state string, userData *common.UserData) error {
	var body io.Reader
	if userData != nil {
		b := new(bytes.Buffer)
		err := json.NewEncoder(b).Encode(userData)
		if err != nil {
			return err
		}
		body = b
	}
	resp, err := c.httpPost(fmt.Sprintf("%v/update?name=%v&owner=%v&state=%v",
		c.url, name, c.owner, state), "application/json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s, status code %v updating %s", resp.Status, resp.StatusCode, name)
	}
	return nil
}

func (c *Client) reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	rmap := make(map[string]string)
	resp, err := c.httpPost(fmt.Sprintf("%v/reset?type=%v&state=%v&expire=%v&dest=%v",
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
	resp, err := c.httpGet(fmt.Sprintf("%v/metric?type=%v", c.url, rtype))
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

func (c *Client) httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.httpDo(req)
}

func (c *Client) httpPost(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.httpDo(req)
}

func (c *Client) httpDo(req *http.Request) (*http.Response, error) {
	// Always bump the retryCount by 1 in order to equal the actual number of
	// attempts. For example, if a retryCount of 2 is specified, the intent
	// is for three attempts -- the initial attempt with two retries in case
	// the initial attempt times out.
	retryCount := c.RetryCount + 1
	retryWait := c.RetryWait
	i := uint(0)
	for {
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			if err2, ok := err.(*url.Error); ok && err2.Timeout() {
				if i < retryCount-1 {
					i++
					time.Sleep(retryWait)
					continue
				}
			}
			return nil, err
		}
		return res, nil
	}
}
