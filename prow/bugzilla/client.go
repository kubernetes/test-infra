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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Client interface {
	Endpoint() string
	GetBug(id int) (*Bug, error)
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

func (c *client) GetBug(id int) (*Bug, error) {
	logger := c.logger.WithFields(logrus.Fields{"method": "GetBug", "id": id})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	if apiKey := c.getAPIKey(); len(apiKey) > 0 {
		req.Header.Set("X-BUGZILLA-API-KEY", string(apiKey))
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
