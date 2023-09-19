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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/version"
)

type Client struct {
	host       string
	httpClient *http.Client
}

func NewClient(host string, timeout time.Duration) *Client {
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
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
	return c.httpClient.Do(req)
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
func (c *Client) GetInRepoConfig(identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) (*config.ProwYAML, error) {
	refs := prowapi.Refs{}

	orgRepo := config.NewOrgRepo(identifier)
	refs.Org = orgRepo.Org
	refs.Repo = orgRepo.Repo

	var err error
	refs.BaseSHA, err = baseSHAGetter()
	if err != nil {
		return nil, err
	}

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

	return c.GetProwYAML(&refs)
}
