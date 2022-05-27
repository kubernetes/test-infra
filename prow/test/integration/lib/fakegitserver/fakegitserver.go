/*
Copyright 2022 The Kubernetes Authors.

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

package fakegitserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	host       string
	httpClient *http.Client
}

type RepoSetup struct {
	// Name of the Git repo. It will get a ".git" appended to it and be
	// initialized underneath o.gitReposParentDir.
	Name string `json:"name"`
	// Script to execute. This script runs inside the repo to perform any
	// additional repo setup tasks. This script is executed by /bin/sh.
	Script string `json:"script"`
	// Whether to create the repo at the path (o.gitReposParentDir + name +
	// ".git") even if a file (directory) exists there already. This basically
	// does a 'rm -rf' of the folder first.
	Overwrite bool `json:"overwrite"`
}

func NewClient(host string, timeout time.Duration) *Client {
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) do(method, endpoint string, payload []byte, params map[string]string) (*http.Response, error) {
	baseURL := fmt.Sprintf("%s/%s", c.host, endpoint)
	req, err := http.NewRequest(method, baseURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	q := req.URL.Query()
	for key, val := range params {
		q.Set(key, val)
	}
	req.URL.RawQuery = q.Encode()
	return c.httpClient.Do(req)
}

// setupRepo sends a POST request with the RepoSetup contents. This is a
// client-side function.
func (c *Client) SetupRepo(repoSetup RepoSetup) error {
	buf, err := json.Marshal(repoSetup)
	if err != nil {
		return fmt.Errorf("could not marshal %v", repoSetup)
	}

	resp, err := c.do(http.MethodPost, "setup-repo", buf, nil)
	if err != nil {
		return errors.New("FGS repo setup failed")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("got %v response", resp.StatusCode)
	}
	return nil
}
