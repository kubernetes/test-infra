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
	"flag"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/githubeventserver"
	"k8s.io/test-infra/prow/phony"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/repoowners"
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

var repoLevelSecret = `
'*':
  - value: key1
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
foo/bar:
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key6
    created_at: 2020-10-02T15:00:00Z
`

var orgLevelSecret = `
'*':
  - value: key1
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
foo:
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key4
    created_at: 2020-10-02T15:00:00Z
`

var globalSecret = `
'*':
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
`

var missingMatchingSecret = `
somerandom:
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
`

var secretInOldFormat = `123abc`

// TestHook sets up a hook.Server and then sends a fake webhook at it. It then
// ensures that a fake plugin is called.
func TestHook(t *testing.T) {
	called := make(chan bool, 1)
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
	clientAgent := &plugins.ClientAgent{
		GitHubClient:   github.NewFakeClient(),
		OwnersClient:   repoowners.NewClient(nil, nil, func(org, repo string) bool { return false }, func(org, repo string) bool { return false }, func() config.OwnersDirBlacklist { return config.OwnersDirBlacklist{} }, ownersconfig.FakeResolver),
		BugzillaClient: &bugzilla.Fake{},
	}
	var testcases = []struct {
		name           string
		secret         []byte
		tokenGenerator func() []byte
		shouldSucceed  bool
	}{
		{
			name:   "Token present at repository level.",
			secret: []byte("123abc"),
			tokenGenerator: func() []byte {
				return []byte(repoLevelSecret)
			},
			shouldSucceed: true,
		},
		{
			name:   "Token present at org level.",
			secret: []byte("123abc"),
			tokenGenerator: func() []byte {
				return []byte(orgLevelSecret)
			},
			shouldSucceed: true,
		},
		{
			name:   "Token present at global level.",
			secret: []byte("123abc"),
			tokenGenerator: func() []byte {
				return []byte(globalSecret)
			},
			shouldSucceed: true,
		},
		{
			name:   "Token not matching anywhere (wildcard token missing).",
			secret: []byte("123abc"),
			tokenGenerator: func() []byte {
				return []byte(missingMatchingSecret)
			},
			shouldSucceed: false,
		},
		{
			name:   "Secret in old format.",
			secret: []byte("123abc"),
			tokenGenerator: func() []byte {
				return []byte(secretInOldFormat)
			},
			shouldSucceed: true,
		},
	}

	for _, tc := range testcases {
		t.Logf("Running scenario %q", tc.name)

		metrics := githubeventserver.NewMetrics()
		fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		var geo githubeventserver.Options
		geo.Bind(fs)
		geo.Metrics = metrics

		server := &Server{
			ClientAgent: clientAgent,
			Plugins:     pa,
			ConfigAgent: ca,
			RepoEnabled: func(org, repo string) bool { return true },
			Metrics:     metrics,
		}

		eventServer := githubeventserver.New(geo, tc.tokenGenerator, logrus.NewEntry(logrus.New()))
		eventServer.RegisterExternalPlugins(pa.Config().ExternalPlugins)
		eventServer.RegisterIssueEventHandler(server.HandleIssueEvent)

		s := httptest.NewServer(eventServer.GetServeMuxHandler())
		defer s.Close()

		if err := phony.SendHook(s.URL, "issues", payload, tc.secret); (err != nil) == tc.shouldSucceed {
			t.Fatalf("Error sending hook: %v", err)
		}
	}

	select {
	case <-called: // All good.
	case <-time.After(time.Second):
		t.Error("Plugin not called after one second.")
	}
}
