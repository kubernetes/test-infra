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
	"github.com/satori/go.uuid"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	maxRetries = 8
	retryDelay = 2 * time.Second
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
	client  *http.Client
	baseURL string
	user    string
	token   string
	dry     bool
}

type BuildRequest struct {
	JobName  string
	PRNumber int
	BaseRef  string
	BaseSHA  string
	PullSHA  string
}

type Build struct {
	jobName  string
	pr       int
	id       string
	queueURL *url.URL
}

func NewClient(url, user, token string) *Client {
	return &Client{
		baseURL: url,
		user:    user,
		token:   token,
		client:  &http.Client{},
		dry:     false,
	}
}

func NewDryRunClient(url, user, token string) *Client {
	return &Client{
		baseURL: url,
		user:    user,
		token:   token,
		client:  &http.Client{},
		dry:     true,
	}
}

// Retry on transport failures. Does not retry on 500s.
func (c *Client) request(method, path string) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := retryDelay
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(method, path)
		if err == nil {
			break
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return resp, err
}

func (c *Client) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.token)
	return c.client.Do(req)
}

// Build triggers the job on Jenkins with an ID parameter that will let us
// track it.
func (c *Client) Build(br BuildRequest) (*Build, error) {
	if c.dry {
		return &Build{}, nil
	}
	buildID := uuid.NewV1().String()
	u, err := url.Parse(fmt.Sprintf("%s/job/%s/buildWithParameters", c.baseURL, br.JobName))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("buildId", buildID)
	// These two are provided for backwards-compatability with scripts that
	// used the ghprb plugin.
	q.Set("ghprbPullId", strconv.Itoa(br.PRNumber))
	q.Set("ghprbTargetBranch", br.BaseRef)

	q.Set("PULL_NUMBER", strconv.Itoa(br.PRNumber))
	q.Set("PULL_BASE_REF", br.BaseRef)
	q.Set("PULL_BASE_SHA", br.BaseSHA)
	q.Set("PULL_PULL_SHA", br.PullSHA)
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
		jobName:  br.JobName,
		pr:       br.PRNumber,
		id:       buildID,
		queueURL: loc,
	}, nil
}

// Enqueued returns whether or not the given build is in Jenkins' build queue.
func (c *Client) Enqueued(b *Build) (bool, error) {
	if c.dry {
		return false, nil
	}
	u := fmt.Sprintf("%s/queue/api/json", c.baseURL)
	resp, err := c.request(http.MethodGet, u)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("response not 2XX: %s", resp.Status)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	queue := struct {
		Items []struct {
			Actions []struct {
				Parameters []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"parameters"`
			} `json:"actions"`
		} `json:"items"`
	}{}
	err = json.Unmarshal(buf, &queue)
	if err != nil {
		return false, err
	}
	for _, item := range queue.Items {
		for _, action := range item.Actions {
			for _, p := range action.Parameters {
				if p.Name == "buildId" && p.Value == b.id {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// Status returns the current status of the build.
func (c *Client) Status(b *Build) (*Status, error) {
	if c.dry {
		return &Status{
			Building: false,
			Success:  true,
		}, nil
	}
	u := fmt.Sprintf("%s/job/%s/api/json?tree=builds[number,result,actions[parameters[name,value]]]", c.baseURL, b.jobName)
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
				if p.Name == "buildId" && p.Value == b.id {
					if build.Result == nil {
						return &Status{Building: true, Number: build.Number}, nil
					} else {
						return &Status{
							Building: false,
							Success:  *build.Result == "SUCCESS",
							Number:   build.Number,
						}, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("did not find build %s", b.id)
}
