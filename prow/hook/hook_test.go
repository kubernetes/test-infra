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

package hook

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/phony"
	"k8s.io/test-infra/prow/plugins"
)

var ice = github.IssueCommentEvent{
	Action: "reopened",
	Repo: github.Repo{
		Owner: github.User{
			Login: "foo",
		},
		Name:     "bar",
		FullName: "foo/bar",
	},
}

// TestHook sets up a hook.Server and then sends a fake webhook at it. It then
// ensures that a fake plugin is called.
func TestHook(t *testing.T) {
	called := make(chan bool, 1)
	secret := []byte("123abc")
	payload, err := json.Marshal(&ice)
	if err != nil {
		t.Fatalf("Marshalling ICE: %v", err)
	}
	plugins.RegisterIssueHandler(
		"baz",
		func(pc plugins.Agent, ie github.IssueEvent) error {
			called <- true
			return nil
		},
		nil,
	)
	pa := &plugins.ConfigAgent{}
	pa.Set(&plugins.Configuration{Plugins: map[string][]string{"foo/bar": {"baz"}}})
	ca := &config.Agent{}
	clientAgent := &plugins.ClientAgent{}
	metrics := NewMetrics()

	getSecret := func() []byte {
		return []byte("123abc")
	}

	s := httptest.NewServer(&Server{
		ClientAgent:    clientAgent,
		Plugins:        pa,
		ConfigAgent:    ca,
		Metrics:        metrics,
		TokenGenerator: getSecret,
	})
	defer s.Close()
	if err := phony.SendHook(s.URL, "issues", payload, secret); err != nil {
		t.Fatalf("Error sending hook: %v", err)
	}
	select {
	case <-called: // All good.
	case <-time.After(time.Second):
		t.Error("Plugin not called after one second.")
	}
}
