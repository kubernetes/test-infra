/*
Copyright 2023 The Kubernetes Authors.

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

package moonraker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/wait"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/version"
)

// The the Client needs a Config agent client. Here we require that the Agent
// type fits the prowConfigAgentClient interface, which requires a Config()
// method to retrieve the current Config. Tests can use a fake Config agent
// instead of the real one.
var _ prowConfigAgentClient = (*config.Agent)(nil)

type prowConfigAgentClient interface {
	Config() *config.Config
}

type Client struct {
	host        string
	configAgent prowConfigAgentClient

	sync.Mutex // protects below
	httpClient *http.Client
}

func NewClient(host string, configAgent prowConfigAgentClient) (*Client, error) {
	c := Client{
		host: host,
		httpClient: &http.Client{
			Timeout: configAgent.Config().Moonraker.ClientTimeout.Duration,
		},
		configAgent: configAgent,
	}

	// isMoonrakerUp is a ConditionFunc (see
	// https://pkg.go.dev/k8s.io/apimachinery/pkg/util/wait#ConditionFunc).
	isMoonrakerUp := func() (bool, error) {
		if err := c.Ping(); err != nil {
			return false, nil
		} else {
			return true, nil
		}
	}

	pollLoopTimeout := 15 * time.Second
	pollInterval := 500 * time.Millisecond
	if err := wait.Poll(pollInterval, pollLoopTimeout, isMoonrakerUp); err != nil {
		return nil, errors.New("timed out waiting for Moonraker to be available")
	}

	return &c, nil
}

func (c *Client) do(method, path string, payload []byte, params map[string]string) (*http.Response, error) {
	baseURL, err := url.JoinPath(c.host, path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, baseURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	req.Header.Add("User-Agent", version.UserAgent())
	q := req.URL.Query()
	for key, val := range params {
		q.Set(key, val)
	}
	req.URL.RawQuery = q.Encode()
	return c.maybeUpdatedHttpClient().Do(req)
}

// maybeUpdatedHttpClient returns a new HTTP client with a new one (with
// a new timeout) if the provided timeout does not match the value already used
// for the current HTTP client.
func (c *Client) maybeUpdatedHttpClient() *http.Client {
	c.Lock()
	defer c.Unlock()
	timeout := c.configAgent.Config().Moonraker.ClientTimeout.Duration

	if c.httpClient.Timeout != timeout {
		c.httpClient = &http.Client{Timeout: timeout}
	}

	return c.httpClient
}

func (c *Client) Ping() error {
	resp, err := c.do(http.MethodGet, PathPing, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
		return nil
	}

	return fmt.Errorf("invalid status code %d", resp.StatusCode)
}

// GetProwYAML returns the inrepoconfig contents for a repo, based on the Refs
// struct as the input. From the Refs, Moonraker can determine the org/repo,
// BaseSHA, and the Pulls[] (additional refs of each PR, if any) to grab the
// inrepoconfig contents.
func (c *Client) GetProwYAML(refs *prowapi.Refs) (*config.ProwYAML, error) {
	payload := payload{
		Refs: *refs,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not marshal %v", payload)
	}

	resp, err := c.do(http.MethodPost, PathGetInrepoconfig, buf, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("got %v response", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	prowYAML := config.ProwYAML{}
	err = json.Unmarshal(body, &prowYAML)
	if err != nil {
		logrus.WithError(err).Info("unable to unmarshal getInrepoconfig request")
		return nil, err
	}

	return &prowYAML, nil
}

// GetInRepoConfig just wraps around GetProwYAML(), converting the input
// parameters into a prowapi.Refs{} type.
//
// Importantly, it also does defaulting of the retrieved jobs. Defaulting is
// required because the Presubmit and Postsubmit job types have private fields
// in them that would not be serialized into JSON when sent over from the
// server. So the defaulting has to be done client-side.
func (c *Client) GetInRepoConfig(identifier, baseBranch string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) (*config.ProwYAML, error) {
	refs := prowapi.Refs{}

	orgRepo := config.NewOrgRepo(identifier)
	refs.Org = orgRepo.Org
	refs.Repo = orgRepo.Repo

	var err error
	refs.BaseSHA, err = baseSHAGetter()
	if err != nil {
		return nil, err
	}

	refs.BaseRef = baseBranch

	pulls := []prowapi.Pull{}
	for _, headSHAGetter := range headSHAGetters {
		headSHA, err := headSHAGetter()
		if err != nil {
			return nil, err
		}
		pulls = append(pulls, prowapi.Pull{
			SHA: headSHA,
		})
	}
	refs.Pulls = pulls

	prowYAML, err := c.GetProwYAML(&refs)
	if err != nil {
		return nil, err
	}

	cfg := c.configAgent.Config()
	if err := config.DefaultAndValidateProwYAML(cfg, prowYAML, identifier); err != nil {
		return nil, err
	}

	return prowYAML, nil
}

func (c *Client) GetPresubmits(identifier, baseBranch string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error) {
	prowYAML, err := c.GetInRepoConfig(identifier, baseBranch, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	config := c.configAgent.Config()
	return append(config.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

func (c *Client) GetPostsubmits(identifier, baseBranch string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Postsubmit, error) {
	prowYAML, err := c.GetInRepoConfig(identifier, baseBranch, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	config := c.configAgent.Config()
	return append(config.GetPostsubmitsStatic(identifier), prowYAML.Postsubmits...), nil
}
