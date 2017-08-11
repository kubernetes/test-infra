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

package mungers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"

	"k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"

	githubapi "github.com/google/go-github/github"
)

const milestoneTestBotName = "test-bot"

// TestMilestoneMaintainer validates that notification state can be
// determined and applied to an issue.  Comprehensive testing is left
// to TestNotificationState.
//
// TODO(marun) Enable testing of comment deletion
func TestMilestoneMaintainer(t *testing.T) {
	activeMilestone := "v1.8"
	milestone := &githubapi.Milestone{Title: &activeMilestone, Number: intPtr(1)}
	m := MilestoneMaintainer{
		activeMilestone: activeMilestone,
		gracePeriodDays: 3,
	}

	issue := github_test.Issue("user", 1, []string{}, false)
	issue.Milestone = milestone

	config := &github.Config{Org: "o", Project: "r"}
	client, server, mux := github_test.InitServer(t, issue, nil, nil, nil, nil, nil, nil)
	config.SetClient(client)

	path := fmt.Sprintf("/repos/%s/%s/issues/%d", config.Org, config.Project, *issue.Number)

	mux.HandleFunc(fmt.Sprintf("%s/labels", path), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		out := []githubapi.Label{{}}
		data, err := json.Marshal(out)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		w.Write(data)
	})

	var comments []githubapi.IssueComment
	mux.HandleFunc(fmt.Sprintf("%s/comments", path), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			c := new(githubapi.IssueComment)
			json.NewDecoder(r.Body).Decode(c)
			comments = append(comments, *c)
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(githubapi.IssueComment{})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
			return
		}
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal([]githubapi.IssueComment{})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
			return
		}
		t.Fatalf("Unexpected method: %s", r.Method)
	})

	obj, err := config.GetObject(*issue.Number)
	if err != nil {
		t.Fatal(err)
	}

	m.Munge(obj)

	expectedLabel := milestoneLabelsIncompleteLabel
	if !obj.HasLabel(expectedLabel) {
		t.Fatalf("Issue labels do not include '%s'", expectedLabel)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected comment count of %d, got %d", 1, len(comments))
	}

	server.Close()
}

func milestoneTestComment(label string, createdAt time.Time) *c.Comment {
	n := &c.Notification{
		Name:      milestoneNotifierName,
		Arguments: label,
		Context:   "foo",
	}
	return &c.Comment{
		Body:      stringPtr(n.String()),
		CreatedAt: &createdAt,
	}
}

func milestoneLabelEvents(label string, createdAt time.Time) []*githubapi.IssueEvent {
	return []*githubapi.IssueEvent{
		{
			Event: stringPtr("labeled"),
			Label: &githubapi.Label{
				Name: &label,
			},
			CreatedAt: &createdAt,
			Actor: &githubapi.User{
				Login: stringPtr(milestoneTestBotName),
			},
		},
	}
}

// TestNotificationState validates how the notificationState() method
// responds to the possible label and notification states of issues in
// a milestone.
func TestNotificationState(t *testing.T) {
	createdNow := time.Now()
	createdYesterday := createdNow.Add(-(milestoneWarningPeriod + time.Hour))
	gracePeriod := 3 * day
	createdLongAgo := createdNow.Add(-(gracePeriod + time.Hour))
	tests := map[string]struct {
		// Labels to apply to the test issue
		labels []string
		// Comment to add to the test issue
		comment *c.Comment
		// Events to add to the test issue
		events []*githubapi.IssueEvent
		// munger label expected to have been decided by notificationState
		expectedLabel string
		// description expected to have been decided by notificationState
		expectedDescription string
	}{
		"Label and comment with summary for complete labels": {
			labels:              []string{"kind/bug", "priority/important-soon", "sig/foo"},
			expectedLabel:       milestoneLabelsCompleteLabel,
			expectedDescription: milestoneLabelsComplete,
		},
		"Do nothing for up-to-date summary": {
			labels:        []string{"kind/bug", "priority/important-soon", "sig/foo"},
			comment:       milestoneTestComment(milestoneLabelsComplete, createdNow),
			expectedLabel: milestoneLabelsCompleteLabel,
		},
		"Label and warn for the first time": {
			expectedLabel:       milestoneLabelsIncompleteLabel,
			expectedDescription: milestoneLabelsIncomplete,
		},
		"Do nothing for up-to-date warning": {
			comment:       milestoneTestComment(milestoneLabelsIncomplete, createdNow),
			expectedLabel: milestoneLabelsIncompleteLabel,
		},
		"Warn again after warning interval has elapsed": {
			comment:             milestoneTestComment(milestoneLabelsIncomplete, createdYesterday),
			expectedLabel:       milestoneLabelsIncompleteLabel,
			expectedDescription: milestoneLabelsIncomplete,
		},
		"Non-blocker is removed from milestone after grace period": {
			labels:              []string{milestoneLabelsIncompleteLabel},
			comment:             milestoneTestComment(milestoneLabelsIncomplete, createdYesterday),
			events:              milestoneLabelEvents(milestoneLabelsIncompleteLabel, createdLongAgo),
			expectedLabel:       milestoneRemovedLabel,
			expectedDescription: milestoneRemoved,
		},
		"Blocker is not removed from milestone after grace period": {
			labels:        []string{milestoneLabelsIncompleteLabel, priorityCriticalUrgent},
			comment:       milestoneTestComment(milestoneLabelsIncomplete, createdYesterday),
			events:        milestoneLabelEvents(milestoneLabelsIncompleteLabel, createdLongAgo),
			expectedLabel: milestoneLabelsIncompleteLabel,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			issue := github_test.Issue("user", 1, test.labels, false)
			milestone := &githubapi.Milestone{Title: stringPtr("v1.8"), Number: intPtr(1)}
			issue.Milestone = milestone

			client, server, _ := github_test.InitServer(t, issue, nil, test.events, nil, nil, nil, nil)
			defer server.Close()

			config := &github.Config{Org: "o", Project: "r"}
			config.SetClient(client)
			obj, err := config.GetObject(*issue.Number)
			if err != nil {
				t.Fatal(err)
			}

			label, notifyState := notificationState(obj, test.comment, milestoneTestBotName, gracePeriod)

			if test.expectedLabel != label {
				t.Fatalf("%s: Expected label '%s', got '%s'", testName, test.expectedLabel, label)
			}

			if len(test.expectedDescription) == 0 && notifyState != nil {
				t.Fatalf("%s: Expected no change in notification state", testName)
			}
			if len(test.expectedDescription) != 0 {
				if notifyState == nil {
					t.Fatalf("%s: Expected a change in notification state", testName)
				}
				if test.expectedDescription != notifyState.description {
					t.Fatalf("%s: Expected description %s, got %s", testName, test.expectedDescription, notifyState.description)
				}
			}
		})
	}
}

func TestIgnoreObject(t *testing.T) {
	tests := map[string]struct {
		isPR            bool
		milestone       string
		activeMilestone string
		expectedIgnore  bool
	}{
		"Ignore PR": {
			isPR:           true,
			expectedIgnore: true,
		},
		"Ignore issue without a milestone": {
			expectedIgnore: true,
		},
		"Ignore issue not in active milestone": {
			milestone:       "v1.7",
			activeMilestone: "v1.8",
			expectedIgnore:  true,
		},
		"Do not ignore issue in active milestone": {
			milestone:       "v1.8",
			activeMilestone: "v1.8",
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			issue := github_test.Issue("user", 1, nil, test.isPR)
			issue.Milestone = &githubapi.Milestone{Title: stringPtr(test.milestone), Number: intPtr(1)}
			obj := &github.MungeObject{Issue: issue}

			ignore := ignoreObject(obj, test.activeMilestone)

			if ignore != test.expectedIgnore {
				t.Fatalf("%s: Expected ignore to be %t, got %t", testName, test.expectedIgnore, ignore)
			}
		})

	}
}

func TestUniqueLabelName(t *testing.T) {
	labelMap := map[string]string{
		"foo": "",
		"bar": "",
	}
	tests := map[string]struct {
		labelNames    []string
		expectedLabel string
		expectedErr   bool
	}{
		"Unmatched label set returns empty string": {},
		"Single label match returned": {
			labelNames:    []string{"foo"},
			expectedLabel: "foo",
		},
		"Multiple label matches returns error": {
			labelNames:  []string{"foo", "bar"},
			expectedErr: true,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			labels := github_test.StringsToLabels(test.labelNames)

			label, err := uniqueLabelName(labels, labelMap)

			if label != test.expectedLabel {
				t.Fatalf("%s: Expected label '%s', got '%s'", testName, test.expectedLabel, label)
			}
			if test.expectedErr && err == nil {
				t.Fatalf("%s: Err expected but did not occur", testName)
			}
			if !test.expectedErr && err != nil {
				t.Fatalf("%s: Unexpected error occurred", testName)
			}
		})
	}
}

func TestSigLabelNames(t *testing.T) {
	labels := github_test.StringsToLabels([]string{"sig/foo", "sig/bar", "baz"})
	labelNames := sigLabelNames(labels)
	// Expect labels without sig/ prefix to be filtered out
	expectedLabelNames := []string{"sig/foo", "sig/bar"}
	if !reflect.DeepEqual(expectedLabelNames, labelNames) {
		t.Fatalf("Expected %v, got %v", expectedLabelNames, labelNames)
	}
}
