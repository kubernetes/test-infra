/*
Copyright 2017 The Kubernetes Authors.

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

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger provides an interface to log debug messages.
type Logger interface {
	Debugf(s string, v ...interface{})
}

// Client allows you to send a webhook with a JSON payload. It sets a token
// that can authenticate the connection to the target server.
type Client struct {
	// If logger is non-nil, log all method calls with it.
	logger Logger

	tokenGenerator func(message []byte) (string, error)
	fake           bool
}

// TokenGenerator is a function that takes an unhashed message digest, and
// returns a stringified token to be included in a HTTP header.
type TokenGenerator func(message []byte) (string, error)

// NewClient creates a webhook client. The generated webhooks contain the
// result of `tokenGenerator(digest)` in a "token" HTTP header.
func NewClient(tokenGenerator TokenGenerator) *Client {
	return &Client{
		logger:         logrus.WithField("client", "webhook"),
		tokenGenerator: tokenGenerator,
	}
}

// NewFakeClient returns a client that takes no actions.
func NewFakeClient() *Client {
	return &Client{
		fake: true,
	}
}

func (sl *Client) log(methodName string, args ...interface{}) {
	if sl.logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	sl.logger.Debugf("%s(%s)", methodName, strings.Join(as, ", "))
}

// digest is a unique identifier of the webhook call that is sent to the token
// generator. It is the concatenation of:
//
// 1. the target url
// 2. an RFC3339-formatted timestamp of the current time in the UTC timezone
// 3. the request body
//
// The three fields are separated by nullbytes.
func digest(url, timestamp string, message []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(url)
	buf.WriteByte(0)
	buf.WriteString(timestamp)
	buf.WriteByte(0)
	buf.Write(message)
	return buf.Bytes()
}

// Send runs a POST call to the given url with the given message as body.
// Message must be a `json.Marshaler` or otherwise be processable with
// `json.Marshal`. Context is for cancellation.
func (sl *Client) Send(ctx context.Context, url string, message any) (err error) {
	sl.log("Send", url)
	if sl.fake {
		return nil
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(message); err != nil {
		return fmt.Errorf("failed to encode the message body to JSON: %w", err)
	}
	token, err := sl.tokenGenerator(digest(url, timestamp, body.Bytes()))
	if err != nil {
		return fmt.Errorf("failed to get a token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("failed to create the request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-prow-timestamp", timestamp)
	req.Header.Set("x-prow-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the request: %w", err)
	}

	defer func() {
		// Only return the potential response-body-flush error if no
		// other error has been found
		if _, readErr := io.Copy(io.Discard, resp.Body); err == nil && readErr != nil {
			err = fmt.Errorf("failed to read the response: %w", readErr)
		}

		// Only return the potential response-body-close error if no
		// other error has been found
		if closeErr := resp.Body.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close the response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
