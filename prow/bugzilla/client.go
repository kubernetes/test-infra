/*
Copyright 2019 The Kubernetes Authors.

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

package bugzilla

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Client interface {
	Endpoint() string
	GetBug(id int) (*Bug, error)
	UpdateBug(id int, update BugUpdate) error
}

func NewClient(getAPIKey func() []byte, endpoint string) Client {
	return &client{
		logger:    logrus.WithField("client", "bugzilla"),
		client:    &http.Client{},
		endpoint:  endpoint,
		getAPIKey: getAPIKey,
	}
}

type client struct {
	logger    *logrus.Entry
	client    *http.Client
	endpoint  string
	getAPIKey func() []byte
}

// the client is a Client impl
var _ Client = &client{}

func (c *client) Endpoint() string {
	return c.endpoint
}

// GetBug retrieves a Bug from the server
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#get-bug
func (c *client) GetBug(id int) (*Bug, error) {
	logger := c.logger.WithFields(logrus.Fields{"method": "GetBug", "id": id})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(req, logger)
	if err != nil {
		return nil, err
	}
	var parsedResponse struct {
		Bugs []*Bug `json:"bugs,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsedResponse); err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %v", err)
	}
	if len(parsedResponse.Bugs) != 1 {
		return nil, fmt.Errorf("did not get one bug, but %d: %v", len(parsedResponse.Bugs), parsedResponse)
	}
	return parsedResponse.Bugs[0], nil
}

// UpdateBug updates the fields of a bug on the server
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#update-bug
func (c *client) UpdateBug(id int, update BugUpdate) error {
	logger := c.logger.WithFields(logrus.Fields{"method": "UpdateBug", "id": id, "update": update})
	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update payload: %v", update)
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = c.request(req, logger)
	return err
}

func (c *client) request(req *http.Request, logger *logrus.Entry) ([]byte, error) {
	if apiKey := c.getAPIKey(); len(apiKey) > 0 {
		// some BugZilla servers are too old and can't handle the header.
		// some don't want the query parameter. We can set both and keep
		// everyone happy without negotiating on versions
		req.Header.Set("X-BUGZILLA-API-KEY", string(apiKey))
		values := req.URL.Query()
		values.Add("api_key", string(apiKey))
		req.URL.RawQuery = values.Encode()
	}
	resp, err := c.client.Do(req)
	logger.WithField("response", resp.StatusCode).Debug("Got response from Bugzilla.")
	if err != nil {
		code := -1
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, &requestError{statusCode: code, message: err.Error()}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.WithError(err).Warn("could not close response body")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, &requestError{statusCode: resp.StatusCode, message: fmt.Sprintf("response code %d not %d", resp.StatusCode, http.StatusOK)}
	}
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v", err)
	}
	return raw, nil
}

type requestError struct {
	statusCode int
	message    string
}

func (e requestError) Error() string {
	return e.message
}

func IsNotFound(err error) bool {
	reqError, ok := err.(*requestError)
	if !ok {
		return false
	}
	return reqError.statusCode == http.StatusNotFound
}
