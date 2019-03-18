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

// Package slack is used for basic interaction with Slack.
package slack

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client has methods for interacting with Slack.
type Client struct {
	Config Config
}

// New returns a new Client.
func New(config Config) *Client {
	return &Client{Config: config}
}

// Calls most Slack API methods by name. If the API is normal but the URL is weird,
// providing a complete https:// URL as the API name also works.
func (c *Client) CallMethod(api string, args interface{}, ret interface{}) error {
	marshalled, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %v", err)
	}
	b := bytes.NewBuffer(marshalled)
	url := api
	if !strings.HasPrefix(url, "https://") {
		url = "https://slack.com/api/" + api
	}
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.Config.AccessToken)
	client := http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to POST message to Slack: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("slack has rate limited us for the next %s seconds", response.Header.Get("Retry-After"))
		}
		return fmt.Errorf("sending message to Slack failed")
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		result := struct {
			OK       bool   `json:"ok"`
			Error    string `json:"error"`
			Metadata struct {
				Messages []string `json:"messages"`
			} `json:"response_metadata"`
		}{}
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("failed to decode JSON response: %v", err)
		}
		if !result.OK {
			return fmt.Errorf("slack call failed: %s (%v)", result.Error, result.Metadata.Messages)
		}
		if ret != nil {
			if err := json.Unmarshal(body, ret); err != nil {
				return fmt.Errorf("slack call succeeded, but failed to unmarshal result: %v", err)
			}
		}
	} else if ret != nil {
		return fmt.Errorf("slack call probably succeeded, but did not get JSON response implied by non-nil ret")
	}
	return nil
}

// SendMessage sends a simple message to Slack.
func (c *Client) SendMessage(message string) error {
	toSend := struct {
		Text string `json:"text"`
	}{message}
	return c.CallMethod(c.Config.WebhookURL, toSend, nil)
}

// VerifySignature verifies the signature on a message from Slack to ensure it is real.
func (c *Client) VerifySignature(body []byte, headers http.Header) error {
	// Algorithm from https://api.slack.com/docs/verifying-requests-from-slack

	expectedSignature := headers.Get("X-Slack-Signature")
	if expectedSignature == "" {
		return fmt.Errorf("X-Slack-Signature missing")
	}
	expectedSignatureBytes, err := hex.DecodeString(strings.TrimPrefix(expectedSignature, "v0="))
	if err != nil {
		return fmt.Errorf("X-Slack-Signature is not a valid hex string")
	}

	// Step 2
	tsHeader := headers.Get("X-Slack-Request-Timestamp")
	if tsHeader == "" {
		return fmt.Errorf("X-Slack-Request-Timestamp header missing")
	}
	tsInt, err := strconv.ParseInt(tsHeader, 10, 64)
	if tsHeader == "" {
		return fmt.Errorf("couldn't parse timestamp %q: %v", tsHeader, err)
	}
	ts := time.Unix(tsInt, 0)
	now := time.Now()
	diff := now.Sub(ts)
	if math.Abs(diff.Minutes()) > 5 {
		return fmt.Errorf("clock difference %s too high", diff)
	}

	// Step 3
	sigBase := append([]byte("v0:"+tsHeader+":"), body...)
	h := hmac.New(sha256.New, []byte(c.Config.SigningSecret))
	_, _ = h.Write(sigBase)
	ourSignature := h.Sum(nil)
	if !hmac.Equal(ourSignature, expectedSignatureBytes) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// EscapeMessage escapes special characters in Slack messages.
func EscapeMessage(s string) string {
	return strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;").Replace(s)
}
