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

package phony

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/test-infra/prow/github"
)

// SendHook sends a GitHub event of type eventType to the provided address.
func SendHook(address, eventType string, payload, hmac []byte) error {
	req, err := http.NewRequest(http.MethodPost, address, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-GitHub-Event", eventType)
	req.Header.Set("X-GitHub-Delivery", "GUID")
	req.Header.Set("X-Hub-Signature", github.PayloadSignature(payload, hmac))
	req.Header.Set("content-type", "application/json")

	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response from hook has status %d and body %s", resp.StatusCode, string(bytes.TrimSpace(rb)))
	}
	return nil
}
