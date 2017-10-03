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
	"strings"
	"time"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	maxRetries = 5
	retryDelay = 100 * time.Millisecond
	buildID    = "buildId"
)

const (
	Succeess = "SUCCESS"
	Failure  = "FAILURE"
	Aborted  = "ABORTED"
)

type Logger interface {
	Debugf(s string, v ...interface{})
}

type JenkinsBuild struct {
	Actions []struct {
		Parameters []struct {
			Name string `json:"name"`
			// This needs to be an interface so we won't clobber
			// json unmarshaling when the Jenkins job has more
			// parameter types than strings.
			Value interface{} `json:"value"`
		} `json:"parameters"`
	} `json:"actions"`
	Task struct {
		// Used for tracking unscheduled builds for jobs.
		Name string `json:"name"`
	} `json:"task"`
	Number   int     `json:"number"`
	Result   *string `json:"result"`
	enqueued bool
}

func (jb *JenkinsBuild) IsRunning() bool {
	return jb.Result == nil
}

func (jb *JenkinsBuild) IsSuccess() bool {
	return jb.Result != nil && *jb.Result == Succeess
}

func (jb *JenkinsBuild) IsFailure() bool {
	return jb.Result != nil && (*jb.Result == Failure || *jb.Result == Aborted)
}

func (jb *JenkinsBuild) IsEnqueued() bool {
	return jb.enqueued
}

func (jb *JenkinsBuild) BuildID() string {
	for _, action := range jb.Actions {
		for _, p := range action.Parameters {
			if p.Name == buildID {
				// This is not safe as far as Go is concerned. Consider
				// stop using Jenkins if this ever breaks.
				return p.Value.(string)
			}
		}
	}
	return ""
}

type Client struct {
	// If Logger is non-nil, log all method calls with it.
	Logger Logger

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

func NewClient(url string, authConfig *AuthConfig) *Client {
	return &Client{
		baseURL:    url,
		authConfig: authConfig,
		client:     &http.Client{},
	}
}

func (c *Client) log(methodName string, args ...interface{}) {
	if c.Logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	c.Logger.Debugf("%s(%s)", methodName, strings.Join(as, ", "))
}

func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.request(http.MethodGet, path)
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
	return buf, nil
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
		} else if err == nil && retries+1 < maxRetries {
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

// Build triggers a Jenkins build for the provided ProwJob. The name of
// the ProwJob is going to be used as the buildId parameter that will help
// us track the build before it's scheduled by Jenkins.
func (c *Client) Build(pj *kube.ProwJob) error {
	c.log(fmt.Sprintf("Build (type=%s job=%s buildId=%s)", pj.Spec.Type, pj.Spec.Job, pj.Metadata.Name))
	u, err := url.Parse(fmt.Sprintf("%s/job/%s/buildWithParameters", c.baseURL, pj.Spec.Job))
	if err != nil {
		return err
	}
	env := pjutil.EnvForSpec(pj.Spec)
	env[buildID] = pj.Metadata.Name

	q := u.Query()
	for key, value := range env {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	resp, err := c.request(http.MethodPost, u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}

// ListJenkinsBuilds returns a list of all in-flight builds for the
// provided jobs. Both running and unscheduled builds will be returned.
func (c *Client) ListJenkinsBuilds(jobs map[string]struct{}) (map[string]JenkinsBuild, error) {
	jenkinsBuilds := make(map[string]JenkinsBuild)

	// Get queued builds.
	queuePath := "/queue/api/json?tree=items[task[name],actions[parameters[name,value]]]"
	c.log("ListJenkinsBuilds", queuePath)
	queueURL := fmt.Sprintf("%s%s", c.baseURL, queuePath)
	data, err := c.get(queueURL)
	if err != nil {
		return nil, fmt.Errorf("cannot list jenkins builds from the queue: %v", err)
	}
	page := struct {
		QueuedBuilds []JenkinsBuild `json:"items"`
	}{}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	for _, jb := range page.QueuedBuilds {
		buildID := jb.BuildID()
		// Ignore builds with missing buildId parameters.
		if buildID == "" {
			continue
		}
		// Ignore builds we didn't ask for.
		if _, exists := jobs[jb.Task.Name]; !exists {
			continue
		}
		jb.enqueued = true
		jenkinsBuilds[buildID] = jb
	}

	// Get all running builds for all provided jobs.
	for job := range jobs {
		path := fmt.Sprintf("/job/%s/api/json?tree=builds[number,result,actions[parameters[name,value]]]", job)
		c.log("ListJenkinsBuilds", path)
		u := fmt.Sprintf("%s%s", c.baseURL, path)
		data, err := c.get(u)
		if err != nil {
			return nil, fmt.Errorf("cannot list jenkins builds for job %q: %v", job, err)
		}
		page := struct {
			Builds []JenkinsBuild `json:"builds"`
		}{}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		for _, jb := range page.Builds {
			buildID := jb.BuildID()
			// Ignore builds with missing buildId parameters.
			if buildID == "" {
				continue
			}
			jenkinsBuilds[buildID] = jb
		}
	}

	return jenkinsBuilds, nil
}

func (c *Client) GetLog(job string, build int) ([]byte, error) {
	c.log("GetLog", job, build)
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
