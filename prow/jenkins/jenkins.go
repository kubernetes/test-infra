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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

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

// NotFoundError is returned by the Jenkins client when
// a job does not exist in Jenkins.
type NotFoundError struct {
	e error
}

func (e NotFoundError) Error() string {
	return e.e.Error()
}

// NewNotFoundError creates a new NotFoundError.
func NewNotFoundError(e error) NotFoundError {
	return NotFoundError{e: e}
}

type Logger interface {
	Debugf(s string, v ...interface{})
	Warnf(s string, v ...interface{})
}

type Action struct {
	Parameters []Parameter `json:"parameters"`
}

type Parameter struct {
	Name string `json:"name"`
	// This needs to be an interface so we won't clobber
	// json unmarshaling when the Jenkins job has more
	// parameter types than strings.
	Value interface{} `json:"value"`
}

type JenkinsBuild struct {
	Actions []Action `json:"actions"`
	Task    struct {
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
	// If logger is non-nil, log all method calls with it.
	logger Logger

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
		logger:     logrus.WithField("client", "jenkins"),
		baseURL:    url,
		authConfig: authConfig,
		client:     &http.Client{},
	}
}

func (c *Client) log(methodName string, args ...interface{}) {
	if c.logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	c.logger.Debugf("%s(%s)", methodName, strings.Join(as, ", "))
}

// Get fetches the data found in the provided path. It includes retries
// on transport failures and 500s.
func (c *Client) Get(path string) ([]byte, error) {
	resp, err := c.request(http.MethodGet, fmt.Sprintf("%s%s", c.baseURL, path))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, NewNotFoundError(errors.New(resp.Status))
	}
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
	if c.authConfig != nil {
		if c.authConfig.Basic != nil {
			req.SetBasicAuth(c.authConfig.Basic.User, c.authConfig.Basic.Token)
		}
		if c.authConfig.BearerToken != nil {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authConfig.BearerToken.Token))
		}
	}
	return c.client.Do(req)
}

// Build triggers a Jenkins build for the provided ProwJob. The name of
// the ProwJob is going to be used as the buildId parameter that will help
// us track the build before it's scheduled by Jenkins.
func (c *Client) Build(pj *kube.ProwJob) error {
	return c.BuildFromSpec(&pj.Spec, pj.Metadata.Name)
}

// BuildFromSpec triggers a Jenkins build for the provided ProwJobSpec. The
// name of the ProwJob is going to be used as the buildId parameter that will
// help us track the build before it's scheduled by Jenkins.
func (c *Client) BuildFromSpec(spec *kube.ProwJobSpec, buildId string) error {
	c.log(fmt.Sprintf("Build (type=%s job=%s buildId=%s)", spec.Type, spec.Job, buildId))
	u, err := url.Parse(fmt.Sprintf("%s/job/%s/buildWithParameters", c.baseURL, spec.Job))
	if err != nil {
		return err
	}
	env := pjutil.EnvForSpec(*spec)
	env[buildID] = buildId

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
	data, err := c.Get(queuePath)
	if err != nil {
		return nil, fmt.Errorf("cannot list builds from the queue: %v", err)
	}
	page := struct {
		QueuedBuilds []JenkinsBuild `json:"items"`
	}{}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("cannot unmarshal builds from the queue: %v", err)
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
		data, err := c.Get(path)
		if err != nil {
			// Ignore 404s so we will not block processing the rest of the jobs.
			if _, isNotFound := err.(NotFoundError); isNotFound {
				c.logger.Warnf("cannot list builds for job %q: %v", job, err)
				continue
			}
			return nil, fmt.Errorf("cannot list builds for job %q: %v", job, err)
		}
		page := struct {
			Builds []JenkinsBuild `json:"builds"`
		}{}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("cannot unmarshal builds for job %q: %v", job, err)
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

// Abort aborts the provided Jenkins build for job. Only running
// builds are aborted.
func (c *Client) Abort(job string, build *JenkinsBuild) error {
	if build.IsEnqueued() {
		return fmt.Errorf("aborting enqueued builds is not supported (tried to abort a build for %s)", job)
	}

	c.log("Abort", job, build.Number)
	u := fmt.Sprintf("%s/job/%s/%d/stop", c.baseURL, job, build.Number)
	resp, err := c.request(http.MethodPost, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("response not 2XX: %s: (%s)", resp.Status, u)
	}
	return nil
}
