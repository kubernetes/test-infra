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

package github

import (
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

// ValidateWebhook ensures that the provided request conforms to the
// format of a Github webhook and the payload can be validated with
// the provided hmac secret. It returns the event type, the event guid,
// the payload of the request, and whether the webhook is valid or not.
func ValidateWebhook(w http.ResponseWriter, r *http.Request, hmacSecret []byte) (string, string, []byte, bool) {
	defer r.Body.Close()

	// Our health check uses GET, so just kick back a 200.
	if r.Method == http.MethodGet {
		return "", "", nil, false
	}

	// Header checks: It must be a POST with an event type and a signature.
	if r.Method != http.MethodPost {
		resp := "405 Method not allowed"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusMethodNotAllowed)
		return "", "", nil, false
	}
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		resp := "400 Bad Request: Missing X-GitHub-Event Header"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	eventGUID := r.Header.Get("X-GitHub-Delivery")
	if eventGUID == "" {
		resp := "400 Bad Request: Missing X-GitHub-Delivery Header"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	sig := r.Header.Get("X-Hub-Signature")
	if sig == "" {
		resp := "403 Forbidden: Missing X-Hub-Signature"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusForbidden)
		return "", "", nil, false
	}
	contentType := r.Header.Get("content-type")
	if contentType != "application/json" {
		resp := "400 Bad Request: Hook only accepts content-type: application/json - please reconfigure this hook on GitHub"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		resp := "500 Internal Server Error: Failed to read request body"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusInternalServerError)
		return "", "", nil, false
	}

	// Validate the payload with our HMAC secret.
	if !ValidatePayload(payload, sig, hmacSecret) {
		resp := "403 Forbidden: Invalid X-Hub-Signature"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusForbidden)
		return "", "", nil, false
	}

	return eventType, eventGUID, payload, true
}
