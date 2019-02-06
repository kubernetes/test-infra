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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const (
	// Maximum retries for a request to Jenkins.
	// Retries on transport failures and 500s.
	maxRetries = 5
	// Backoff delay used after a request retry.
	// Doubles on every retry.
	retryDelay = 100 * time.Millisecond
	// Key for unique build number across Jenkins builds.
	// Used for allowing tools to group artifacts in GCS.
	statusBuildID = "BUILD_ID"
	// Key for unique build number across Jenkins builds.
	// Used for correlating Jenkins builds to ProwJobs.
	prowJobID = "PROW_JOB_ID"
)

const (
	success = "SUCCESS"
	failure = "FAILURE"
	aborted = "ABORTED"
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

// Action holds a list of parameters
type Action struct {
	Parameters []Parameter `json:"parameters"`
}

// Parameter configures some aspect of the job.
type Parameter struct {
	Name string `json:"name"`
	// This needs to be an interface so we won't clobber
	// json unmarshaling when the Jenkins job has more
	// parameter types than strings.
	Value interface{} `json:"value"`
}

// Build holds information about an instance of a jenkins job.
type Build struct {
	Actions []Action `json:"actions"`
	Task    struct {
		// Used for tracking unscheduled builds for jobs.
		Name string `json:"name"`
	} `json:"task"`
	Number   int     `json:"number"`
	Result   *string `json:"result"`
	enqueued bool
}

// IsRunning means the job started but has not finished.
func (jb *Build) IsRunning() bool {
	return jb.Result == nil
}

// IsSuccess means the job passed
func (jb *Build) IsSuccess() bool {
	return jb.Result != nil && *jb.Result == success
}

// IsFailure means the job completed with problems.
func (jb *Build) IsFailure() bool {
	return jb.Result != nil && *jb.Result == failure
}

// IsAborted means something stopped the job before it could finish.
func (jb *Build) IsAborted() bool {
	return jb.Result != nil && *jb.Result == aborted
}

// IsEnqueued means the job has created but has not started.
func (jb *Build) IsEnqueued() bool {
	return jb.enqueued
}

// ProwJobID extracts the ProwJob identifier for the
// Jenkins build in order to correlate the build with
// a ProwJob. If the build has an empty PROW_JOB_ID
// it didn't start by prow.
func (jb *Build) ProwJobID() string {
	for _, action := range jb.Actions {
		for _, p := range action.Parameters {
			if p.Name == prowJobID {
				value, ok := p.Value.(string)
				if !ok {
					logrus.Errorf("Cannot determine %s value for %#v", p.Name, jb)
					continue
				}
				return value
			}
		}
	}
	return ""
}

// BuildID extracts the build identifier used for
// placing and discovering build artifacts.
// This identifier can either originate from tot
// or the snowflake library, depending on how the
// Jenkins operator is configured to run.
// We return an empty string if we are dealing with
// a build that does not have the ProwJobID set
// explicitly, as in that case the Jenkins build has
// not started by prow.
func (jb *Build) BuildID() string {
	var buildID string
	hasProwJobID := false
	for _, action := range jb.Actions {
		for _, p := range action.Parameters {
			hasProwJobID = hasProwJobID || p.Name == prowJobID
			if p.Name == statusBuildID {
				value, ok := p.Value.(string)
				if !ok {
					logrus.Errorf("Cannot determine %s value for %#v", p.Name, jb)
					continue
				}
				buildID = value
			}
		}
	}

	if !hasProwJobID {
		return ""
	}
	return buildID
}

// Client can interact with jenkins to create/manage builds.
type Client struct {
	// If logger is non-nil, log all method calls with it.
	logger *logrus.Entry
	dryRun bool

	client     *http.Client
	baseURL    string
	authConfig *AuthConfig

	metrics *ClientMetrics
}

// AuthConfig configures how we auth with Jenkins.
// Only one of the fields will be non-nil.
type AuthConfig struct {
	// Basic is used for doing basic auth with Jenkins.
	Basic *BasicAuthConfig
	// BearerToken is used for doing oauth-based authentication
	// with Jenkins. Works ootb with the Openshift Jenkins image.
	BearerToken *BearerTokenAuthConfig
	// CSRFProtect ensures the client will acquire a CSRF protection
	// token from Jenkins to use it in mutating requests. Required
	// for masters that prevent cross site request forgery exploits.
	CSRFProtect bool
	// csrfToken is the token acquired from Jenkins for CSRF protection.
	// Needs to be used as the header value in subsequent mutating requests.
	csrfToken string
	// csrfRequestField is a key acquired from Jenkins for CSRF protection.
	// Needs to be used as the header key in subsequent mutating requests.
	csrfRequestField string
}

// BasicAuthConfig authenticates with jenkins using user/pass.
type BasicAuthConfig struct {
	User     string
	GetToken func() []byte
}

// BearerTokenAuthConfig authenticates jenkins using an oauth bearer token.
type BearerTokenAuthConfig struct {
	GetToken func() []byte
}

// NewClient instantiates a client with provided values.
//
// url: the jenkins master to connect to.
// dryRun: mutating calls such as starting/aborting a build will be skipped.
// tlsConfig: configures client transport if set, may be nil.
// authConfig: configures the client to connect to Jenkins via basic auth/bearer token
//             and optionally enables csrf protection
// logger: creates a standard logger if nil.
// metrics: gathers prometheus metrics for the Jenkins client if set.
func NewClient(
	url string,
	dryRun bool,
	tlsConfig *tls.Config,
	authConfig *AuthConfig,
	logger *logrus.Entry,
	metrics *ClientMetrics,
) (*Client, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	c := &Client{
		logger:     logger.WithField("client", "jenkins"),
		dryRun:     dryRun,
		baseURL:    url,
		authConfig: authConfig,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		metrics: metrics,
	}
	if tlsConfig != nil {
		c.client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}
	if c.authConfig.CSRFProtect {
		if err := c.CrumbRequest(); err != nil {
			return nil, fmt.Errorf("cannot get Jenkins crumb: %v", err)
		}
	}
	return c, nil
}

// CrumbRequest requests a CSRF protection token from Jenkins to
// use it in subsequent requests. Required for Jenkins masters that
// prevent cross site request forgery exploits.
func (c *Client) CrumbRequest() error {
	if c.authConfig.csrfToken != "" && c.authConfig.csrfRequestField != "" {
		return nil
	}
	c.logger.Debug("CrumbRequest")
	data, err := c.GetSkipMetrics("/crumbIssuer/api/json")
	if err != nil {
		return err
	}
	crumbResp := struct {
		Crumb             string `json:"crumb"`
		CrumbRequestField string `json:"crumbRequestField"`
	}{}
	if err := json.Unmarshal(data, &crumbResp); err != nil {
		return fmt.Errorf("cannot unmarshal crumb response: %v", err)
	}
	c.authConfig.csrfToken = crumbResp.Crumb
	c.authConfig.csrfRequestField = crumbResp.CrumbRequestField
	return nil
}

// measure records metrics about the provided method, path, and code.
// start needs to be recorded before doing the request.
func (c *Client) measure(method, path string, code int, start time.Time) {
	if c.metrics == nil {
		return
	}
	c.metrics.RequestLatency.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	c.metrics.Requests.WithLabelValues(method, path, fmt.Sprintf("%d", code)).Inc()
}

// GetSkipMetrics fetches the data found in the provided path. It returns the
// content of the response or any errors that occurred during the request or
// http errors. Metrics will not be gathered for this request.
func (c *Client) GetSkipMetrics(path string) ([]byte, error) {
	resp, err := c.request(http.MethodGet, path, nil, false)
	if err != nil {
		return nil, err
	}
	return readResp(resp)
}

// Get fetches the data found in the provided path. It returns the
// content of the response or any errors that occurred during the
// request or http errors.
func (c *Client) Get(path string) ([]byte, error) {
	resp, err := c.request(http.MethodGet, path, nil, true)
	if err != nil {
		return nil, err
	}
	return readResp(resp)
}

func readResp(resp *http.Response) ([]byte, error) {
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

// request executes a request with the provided method and path.
// It retries on transport failures and 500s. measure is provided
// to enable or disable gathering metrics for specific requests
// to avoid high-cardinality metrics.
func (c *Client) request(method, path string, params url.Values, measure bool) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := retryDelay

	urlPath := fmt.Sprintf("%s%s", c.baseURL, path)
	if params != nil {
		urlPath = fmt.Sprintf("%s?%s", urlPath, params.Encode())
	}

	start := time.Now()
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(method, urlPath)
		if err == nil && resp.StatusCode < 500 {
			break
		} else if err == nil && retries+1 < maxRetries {
			resp.Body.Close()
		}
		// Capture the retry in a metric.
		if measure && c.metrics != nil {
			c.metrics.RequestRetries.Inc()
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	if measure && resp != nil {
		c.measure(method, path, resp.StatusCode, start)
	}
	return resp, err
}

// doRequest executes a request with the provided method and path
// exactly once. It sets up authentication if the jenkins client
// is configured accordingly. It's up to callers of this function
// to build retries and error handling.
func (c *Client) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		return nil, err
	}
	if c.authConfig != nil {
		if c.authConfig.Basic != nil {
			req.SetBasicAuth(c.authConfig.Basic.User, string(c.authConfig.Basic.GetToken()))
		}
		if c.authConfig.BearerToken != nil {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authConfig.BearerToken.GetToken()))
		}
		if c.authConfig.CSRFProtect && c.authConfig.csrfRequestField != "" && c.authConfig.csrfToken != "" {
			req.Header.Set(c.authConfig.csrfRequestField, c.authConfig.csrfToken)
		}
	}
	return c.client.Do(req)
}

// Build triggers a Jenkins build for the provided ProwJob. The name of
// the ProwJob is going to be used as the Prow Job ID parameter that will
// help us track the build before it's scheduled by Jenkins.
func (c *Client) Build(pj *kube.ProwJob, buildID string) error {
	c.logger.WithFields(pjutil.ProwJobFields(pj)).Info("Build")
	return c.BuildFromSpec(&pj.Spec, buildID, pj.ObjectMeta.Name)
}

// BuildFromSpec triggers a Jenkins build for the provided ProwJobSpec.
// prowJobID helps us track the build before it's scheduled by Jenkins.
func (c *Client) BuildFromSpec(spec *kube.ProwJobSpec, buildID, prowJobID string) error {
	if c.dryRun {
		return nil
	}
	env, err := downwardapi.EnvForSpec(downwardapi.NewJobSpec(*spec, buildID, prowJobID))
	if err != nil {
		return err
	}
	params := url.Values{}
	for key, value := range env {
		params.Set(key, value)
	}
	path := fmt.Sprintf("/job/%s/buildWithParameters", spec.Job)
	resp, err := c.request(http.MethodPost, path, params, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}

// ListBuilds returns a list of all Jenkins builds for the
// provided jobs (both scheduled and enqueued).
func (c *Client) ListBuilds(jobs []string) (map[string]Build, error) {
	// Get queued builds.
	jenkinsBuilds, err := c.GetEnqueuedBuilds(jobs)
	if err != nil {
		return nil, err
	}

	buildChan := make(chan map[string]Build, len(jobs))
	errChan := make(chan error, len(jobs))
	wg := &sync.WaitGroup{}
	wg.Add(len(jobs))

	// Get all running builds for all provided jobs.
	for _, job := range jobs {
		// Start a goroutine per list
		go func(job string) {
			defer wg.Done()

			builds, err := c.GetBuilds(job)
			if err != nil {
				errChan <- err
			} else {
				buildChan <- builds
			}
		}(job)
	}
	wg.Wait()

	close(buildChan)
	close(errChan)

	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	for builds := range buildChan {
		for id, build := range builds {
			jenkinsBuilds[id] = build
		}
	}

	return jenkinsBuilds, nil
}

// GetEnqueuedBuilds lists all enqueued builds for the provided jobs.
func (c *Client) GetEnqueuedBuilds(jobs []string) (map[string]Build, error) {
	c.logger.Debug("GetEnqueuedBuilds")

	data, err := c.Get("/queue/api/json?tree=items[task[name],actions[parameters[name,value]]]")
	if err != nil {
		return nil, fmt.Errorf("cannot list builds from the queue: %v", err)
	}
	page := struct {
		QueuedBuilds []Build `json:"items"`
	}{}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("cannot unmarshal builds from the queue: %v", err)
	}
	jenkinsBuilds := make(map[string]Build)
	for _, jb := range page.QueuedBuilds {
		prowJobID := jb.ProwJobID()
		// Ignore builds with missing buildID parameters.
		if prowJobID == "" {
			continue
		}
		// Ignore builds for jobs we didn't ask for.
		var exists bool
		for _, job := range jobs {
			if jb.Task.Name == job {
				exists = true
				break
			}
		}
		if !exists {
			continue
		}
		jb.enqueued = true
		jenkinsBuilds[prowJobID] = jb
	}
	return jenkinsBuilds, nil
}

// GetBuilds lists all scheduled builds for the provided job.
// In newer Jenkins versions, this also includes enqueued
// builds (tested in 2.73.2).
func (c *Client) GetBuilds(job string) (map[string]Build, error) {
	c.logger.Debugf("GetBuilds(%v)", job)

	data, err := c.Get(fmt.Sprintf("/job/%s/api/json?tree=builds[number,result,actions[parameters[name,value]]]", job))
	if err != nil {
		// Ignore 404s so we will not block processing the rest of the jobs.
		if _, isNotFound := err.(NotFoundError); isNotFound {
			c.logger.WithError(err).Warnf("Cannot list builds for job %q", job)
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list builds for job %q: %v", job, err)
	}
	page := struct {
		Builds []Build `json:"builds"`
	}{}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("cannot unmarshal builds for job %q: %v", job, err)
	}
	jenkinsBuilds := make(map[string]Build)
	for _, jb := range page.Builds {
		prowJobID := jb.ProwJobID()
		// Ignore builds with missing buildID parameters.
		if prowJobID == "" {
			continue
		}
		jenkinsBuilds[prowJobID] = jb
	}
	return jenkinsBuilds, nil
}

// Abort aborts the provided Jenkins build for job.
func (c *Client) Abort(job string, build *Build) error {
	c.logger.Debugf("Abort(%v %v)", job, build.Number)
	if c.dryRun {
		return nil
	}
	resp, err := c.request(http.MethodPost, fmt.Sprintf("/job/%s/%d/stop", job, build.Number), nil, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("response not 2XX: %s", resp.Status)
	}
	return nil
}
