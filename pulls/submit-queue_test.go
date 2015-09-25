/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pulls

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	github_util "k8s.io/contrib/mungegithub/github"
	github_test "k8s.io/contrib/mungegithub/github/testing"

	"github.com/google/go-github/github"
)

func stringPtr(val string) *string     { return &val }
func timePtr(val time.Time) *time.Time { return &val }
func intPtr(val int) *int              { return &val }

func TestValidateLGTMAfterPush(t *testing.T) {
	tests := []struct {
		issueEvents  []github.IssueEvent
		shouldPass   bool
		lastModified time.Time
	}{
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(9, 0),
			shouldPass:   true,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   false,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(12, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(11, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   true,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(11, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(12, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   true,
		},
	}
	for _, test := range tests {
		config := &github_util.Config{}
		client, server, mux := github_test.InitTest()
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		mux.HandleFunc(fmt.Sprintf("/repos/o/r/issues/1/events"), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.issueEvents)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

			commits, err := config.GetFilledCommits(1)
			if err != nil {
				t.Errorf("Unexpected error getting filled commits: %v", err)
			}

			events, err := config.GetAllEventsForPR(1)
			if err != nil {
				t.Errorf("Unexpected error getting events commits: %v", err)
			}
			lastModifiedTime := github_util.LastModifiedTime(commits)
			lgtmTime := github_util.LabelTime("lgtm", events)

			if lastModifiedTime == nil || lgtmTime == nil {
				t.Errorf("unexpected lastModifiedTime or lgtmTime == nil")
			}

			ok := !lastModifiedTime.After(*lgtmTime)

			if ok != test.shouldPass {
				t.Errorf("expected: %v, saw: %v", test.shouldPass, ok)
			}
		})
		server.Close()
	}
}
