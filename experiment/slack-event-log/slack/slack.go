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
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Slack has methods for interacting with Slack.
type Slack struct {
	Config Config
}

// New returns a new Slack.
func New(config Config) *Slack {
	return &Slack{Config: config}
}

// Calls most Slack API methods by name. If the API is normal but the URL is weird,
// providing a complete https:// URL as the API name also works.
func (slack *Slack) CallMethod(api string, args interface{}) error {
	marshalled, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %v", err)
	}
	b := bytes.NewBuffer(marshalled)
	url := api
	if !strings.HasPrefix(url, "https://") {
		url = "https://slack.com/api/" + api
	}
	response, err := http.Post(url, "application/json", b)
	if err != nil {
		return fmt.Errorf("failed to POST message to Slack: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("sending message to Slack failed")
	}
	return nil
}

// SendMessage sends a simple message to Slack.
func (slack *Slack) SendMessage(message string) error {
	toSend := struct {
		Text string `json:"text"`
	}{message}
	return slack.CallMethod(slack.Config.WebhookURL, toSend)
}

// VerifySignature verifies the signature on a message from Slack to ensure it is real.
func (slack *Slack) VerifySignature(body []byte, headers http.Header) error {
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
	h := hmac.New(sha256.New, []byte(slack.Config.SigningSecret))
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
