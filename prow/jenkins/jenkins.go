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

package jenkins

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

const (
	maxRetries = 5
	retryDelay = 4 * time.Second
)

// Status is a build result from Jenkins. If it is still building then
// Success is meaningless. If it is enqueued then both Success and
// Number are meaningless.
type Status struct {
	Building bool
	Success  bool
	Number   int
}

type Client struct {
	client     *http.Client
	baseURL    string
	authConfig *AuthConfig
}

// AuthConfig configures how we auth with Jenkins.
// Only one of the fields will be non-nil.
type AuthConfig struct {
	Basic       *BasicAuthConfig
	BearerToken *BearerTokenAuthConfig
}

type BasicAuthConfig struct {
	User  string
	Token string
}

type BearerTokenAuthConfig struct {
	Token string
}

type BuildRequest struct {
	ProwJobName string
	JobName     string
	Refs        string
	Environment map[string]string
}

type Build struct {
	JobName  string
	QueueURL *url.URL
}

func NewClient(url string, authConfig *AuthConfig) *Client {
	return &Client{
		baseURL:    url,
		authConfig: authConfig,
		client:     &http.Client{},
	}
}

// Retry on transport failures and 500s.
func (c *Client) request(method, path string) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := retryDelay
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(method, path)
		if err == nil && resp.StatusCode < 500 {
			break
		} else if err == nil {
			resp.Body.Close()
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return resp, err
}

func (c *Client) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		return nil, err
	}
	if c.authConfig.Basic != nil {
		req.SetBasicAuth(c.authConfig.Basic.User, c.authConfig.Basic.Token)
	}
	if c.authConfig.BearerToken != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authConfig.BearerToken.Token))
	}
	return c.client.Do(req)
}

// Build triggers the job on Jenkins with an ID parameter that will let us
// track it.
func (c *Client) Build(br BuildRequest) (*Build, error) {
	u, err := url.Parse(fmt.Sprintf("%s/job/%s/buildWithParameters", c.baseURL, br.JobName))
	if err != nil {
		return nil, err
	}
	br.Environment["buildId"] = br.ProwJobName

	q := u.Query()
	for key, value := range br.Environment {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	resp, err := c.request(http.MethodPost, u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("response not 201: %s", resp.Status)
	}
	loc, err := resp.Location()
	if err != nil {
		return nil, err
	}
	return &Build{
		JobName:  br.JobName,
		QueueURL: loc,
	}, nil
}

// Enqueued returns whether or not the given build is in Jenkins' build queue.
func (c *Client) Enqueued(queueURL string) (bool, error) {
	u := fmt.Sprintf("%sapi/json", queueURL)
	resp, err := c.request(http.MethodGet, u)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("response not 2XX??: %s", resp.Status)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	item := struct {
		Cancelled  bool   `json:"cancelled"`
		Why        string `json:"why"`
		Executable struct {
			Number int `json:"number"`
		} `json:"executable"`
	}{}
	err = json.Unmarshal(buf, &item)
	if err != nil {
		return false, err
	}
	if item.Cancelled {
		return false, fmt.Errorf("job was cancelled: %s", item.Why)
	}
	if item.Executable.Number != 0 {
		return false, nil
	}
	return true, nil
}

// Status returns the current status of the build.
func (c *Client) Status(job, id string) (*Status, error) {
	u := fmt.Sprintf("%s/job/%s/api/json?tree=builds[number,result,actions[parameters[name,value]]]", c.baseURL, job)
	resp, err := c.request(http.MethodGet, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("response not 2XX: %s", resp.Status)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	builds := struct {
		Builds []struct {
			Actions []struct {
				Parameters []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"parameters"`
			} `json:"actions"`
			Number int     `json:"number"`
			Result *string `json:"result"`
		} `json:"builds"`
	}{}
	err = json.Unmarshal(buf, &builds)
	if err != nil {
		return nil, err
	}
	for _, build := range builds.Builds {
		for _, action := range build.Actions {
			for _, p := range action.Parameters {
				if p.Name == "buildId" && p.Value == id {
					if build.Result == nil {
						return &Status{Building: true, Number: build.Number}, nil
					}
					return &Status{
						Building: false,
						Success:  *build.Result == "SUCCESS",
						Number:   build.Number,
					}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("did not find build %s", id)
}

func (c *Client) GetLog(job string, build int) ([]byte, error) {
	u := fmt.Sprintf("%s/job/%s/%d/consoleText", c.baseURL, job, build)
	resp, err := c.request(http.MethodGet, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("response not 2XX: %s: (%s)", resp.Status, u)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
