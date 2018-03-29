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
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"

	githubapi "github.com/google/go-github/github"
)

const milestoneTestBotName = "test-bot"

// TestMilestoneMaintainer validates that notification state can be
// determined and applied to an issue.  Comprehensive testing is left
// to TestNotificationState.
//
// TODO(marun) Enable testing of comment deletion
func TestMilestoneMaintainer(t *testing.T) {
	activeMilestone := "v1.10"
	milestone := &githubapi.Milestone{Title: &activeMilestone, Number: intPtr(1)}
	m := MilestoneMaintainer{
		milestoneModeMap:    map[string]string{activeMilestone: milestoneModeDev},
		approvalGracePeriod: 72 * time.Hour,
		labelGracePeriod:    72 * time.Hour,
		warningInterval:     24 * time.Hour,
	}

	issue := github_test.Issue("user", 1, []string{"kind/bug", "sig/foo", "priority/important-soon"}, false)
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

	expectedLabel := milestoneNeedsApprovalLabel
	if !obj.HasLabel(expectedLabel) {
		t.Fatalf("Issue labels do not include '%s'", expectedLabel)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected comment count of %d, got %d", 1, len(comments))
	}

	expectedBody := `[MILESTONENOTIFIER] Milestone Issue **Needs Approval**

@user @kubernetes/sig-foo-misc


**Action required**: This issue must have the ` + "`status/approved-for-milestone`" + ` label applied by a SIG maintainer. If the label is not applied within 3 days, the issue will be moved out of the v1.10 milestone.
<details>
<summary>Issue Labels</summary>

- ` + "`sig/foo`" + `: Issue will be escalated to these SIGs if needed.
- ` + "`priority/important-soon`" + `: Escalate to the issue owners and SIG owner; move out of milestone after several unsuccessful escalation attempts.
- ` + "`kind/bug`" + `: Fixes a bug discovered during the current release.
</details>
<details>
<summary>Help</summary>
<ul>
 <li><a href="https://git.k8s.io/sig-release/ephemera/issues.md">Additional instructions</a></li>
 <li><a href="https://go.k8s.io/bot-commands">Commands for setting labels</a></li>
</ul>
</details>`
	if comments[0].Body == nil || expectedBody != *comments[0].Body {
		t.Fatalf("Expected Body:\n\n%s\n\nGot:\n\n%s", expectedBody, *comments[0].Body)
	}

	server.Close()
}

// TestNewIssueChangeConfig validates the creation of an IssueChange
// for a given issue state.
func TestNewIssueChangeConfig(t *testing.T) {
	const incompleteLabels = `
_**kind**_: Must specify exactly one of ` + "`kind/bug`, `kind/cleanup` or `kind/feature`." + `
_**sig owner**_: Must specify at least one label prefixed with ` + "`sig/`." + `
`
	const blockerCompleteLabels = `
<summary>Issue Labels</summary>

- ` + "`sig/foo`: Issue will be escalated to these SIGs if needed." + `
- ` + "`priority/critical-urgent`: Never automatically move issue out of a release milestone; continually escalate to contributor and SIG through all available channels." + `
- ` + "`kind/bug`: Fixes a bug discovered during the current release." + `
</details>
`

	const nonBlockerCompleteLabels = `
<summary>Issue Labels</summary>

- ` + "`sig/foo`: Issue will be escalated to these SIGs if needed." + `
- ` + "`priority/important-soon`: Escalate to the issue owners and SIG owner; move out of milestone after several unsuccessful escalation attempts." + `
- ` + "`kind/bug`: Fixes a bug discovered during the current release." + `
</details>
`

	milestoneString := "v1.8"

	munger := &MilestoneMaintainer{
		botName:             milestoneTestBotName,
		labelGracePeriod:    3 * day,
		approvalGracePeriod: 7 * day,
		warningInterval:     day,
		slushUpdateInterval: 3 * day,
		freezeDate:          "the time heck freezes over",
	}

	createdNow := time.Now()
	createdPastLabelGracePeriod := createdNow.Add(-(munger.labelGracePeriod + time.Hour))
	createdPastApprovalGracePeriod := createdNow.Add(-(munger.approvalGracePeriod + time.Hour))
	createdPastSlushUpdateInterval := createdNow.Add(-(munger.slushUpdateInterval + time.Hour))

	tests := map[string]struct {
		// The mode of the munger
		mode string
		// Labels to add to the test issue
		labels []string
		// Whether the test issues milestone labels should be complete
		labelsComplete bool
		// Whether the test issue should be a blocker
		isBlocker bool
		// Whether the test issue should be approved for the milestone
		isApproved bool
		// Events to add to the test issue
		events []*githubapi.IssueEvent
		// Comments to add to the test issue
		comments []*githubapi.IssueComment
		// Sections expected to be enabled
		expectedSections sets.String
		// Expected milestone state
		expectedState milestoneState
		// Expected message body
		expectedBody string
	}{
		"Incomplete labels within grace period": {
			expectedSections: sets.NewString("warnIncompleteLabels"),
			expectedState:    milestoneNeedsLabeling,
			expectedBody: `
**Action required**: This issue requires label changes. If the required changes are not made within 3 days, the issue will be moved out of the v1.8 milestone.
` + incompleteLabels,
		},
		"Incomplete labels outside of grace period": {
			labels:           []string{milestoneLabelsIncompleteLabel},
			events:           milestoneLabelEvents(milestoneLabelsIncompleteLabel, createdPastLabelGracePeriod),
			expectedSections: sets.NewString("removeIncompleteLabels"),
			expectedState:    milestoneNeedsRemoval,
			expectedBody: `
**Important**: This issue was missing labels required for the v1.8 milestone for more than 3 days:

_**kind**_: Must specify exactly one of ` + "`kind/bug`, `kind/cleanup` or `kind/feature`." + `
_**sig owner**_: Must specify at least one label prefixed with ` + "`sig/`.",
		},
		"Incomplete labels outside of grace period, blocker": {
			labels:           []string{milestoneLabelsIncompleteLabel},
			isBlocker:        true,
			events:           milestoneLabelEvents(milestoneLabelsIncompleteLabel, createdPastLabelGracePeriod),
			expectedSections: sets.NewString("warnIncompleteLabels"),
			expectedState:    milestoneNeedsLabeling,
			expectedBody: `
**Action required**: This issue requires label changes.

_**kind**_: Must specify exactly one of ` + "`kind/bug`, `kind/cleanup` or `kind/feature`." + `
_**sig owner**_: Must specify at least one label prefixed with ` + "`sig/`." + `
`,
		},
		"Complete labels, not approved, blocker": {
			labelsComplete:   true,
			isBlocker:        true,
			expectedSections: sets.NewString("summarizeLabels", "warnUnapproved"),
			expectedState:    milestoneNeedsApproval,
			expectedBody: `
**Action required**: This issue must have the ` + "`status/approved-for-milestone`" + ` label applied by a SIG maintainer.
<details>` + blockerCompleteLabels,
		},
		"Complete labels, not approved, non-blocker, within grace period": {
			labelsComplete:   true,
			expectedSections: sets.NewString("summarizeLabels", "warnUnapproved"),
			expectedState:    milestoneNeedsApproval,
			expectedBody: `
**Action required**: This issue must have the ` + "`status/approved-for-milestone`" + ` label applied by a SIG maintainer. If the label is not applied within 7 days, the issue will be moved out of the v1.8 milestone.
<details>` + nonBlockerCompleteLabels,
		},
		"Complete labels, not approved, non-blocker, outside of grace period": {
			labels:           []string{milestoneNeedsApprovalLabel},
			labelsComplete:   true,
			events:           milestoneLabelEvents(milestoneNeedsApprovalLabel, createdPastApprovalGracePeriod),
			expectedSections: sets.NewString("summarizeLabels", "removeUnapproved"),
			expectedState:    milestoneNeedsRemoval,
			expectedBody:     "**Important**: This issue was missing the `status/approved-for-milestone` label for more than 7 days.",
		},
		"dev - Complete labels and approved": {
			labelsComplete:   true,
			isApproved:       true,
			expectedSections: sets.NewString("summarizeLabels"),
			expectedState:    milestoneCurrent,
			expectedBody:     "<details open>" + nonBlockerCompleteLabels,
		},
		"slush - Complete labels, approved, non-blocker, missing in-progress": {
			mode:             milestoneModeSlush,
			labelsComplete:   true,
			isApproved:       true,
			expectedSections: sets.NewString("summarizeLabels", "warnMissingInProgress", "warnNonBlockerRemoval"),
			expectedState:    milestoneNeedsAttention,
			expectedBody: `
**Action required**: During code slush, issues in the milestone should be in progress.
If this issue is not being actively worked on, please remove it from the milestone.
If it is being worked on, please add the ` + "`status/in-progress`" + ` label so it can be tracked with other in-flight issues.

**Note**: If this issue is not resolved or labeled as ` + "`priority/critical-urgent`" + ` by the time heck freezes over it will be moved out of the v1.8 milestone.
<details>` + nonBlockerCompleteLabels,
		},
		"slush - Complete labels, approved, non-blocker": {
			mode:             milestoneModeSlush,
			labels:           []string{"status/in-progress"},
			labelsComplete:   true,
			isApproved:       true,
			expectedSections: sets.NewString("summarizeLabels", "warnNonBlockerRemoval"),
			expectedState:    milestoneCurrent,
			expectedBody: `
**Note**: If this issue is not resolved or labeled as ` + "`priority/critical-urgent`" + ` by the time heck freezes over it will be moved out of the v1.8 milestone.
<details open>` + nonBlockerCompleteLabels,
		},
		"slush - Complete labels, approved, blocker, missing in-progress, update not due": {
			mode:             milestoneModeSlush,
			labelsComplete:   true,
			isApproved:       true,
			isBlocker:        true,
			events:           milestoneLabelEvents(statusApprovedLabel, createdNow),
			comments:         milestoneIssueComments(createdNow),
			expectedSections: sets.NewString("summarizeLabels", "warnMissingInProgress", "warnUpdateInterval"),
			expectedState:    milestoneNeedsAttention,
			expectedBody: `
**Action required**: During code slush, issues in the milestone should be in progress.
If this issue is not being actively worked on, please remove it from the milestone.
If it is being worked on, please add the ` + "`status/in-progress`" + ` label so it can be tracked with other in-flight issues.

**Note**: This issue is marked as ` + "`priority/critical-urgent`" + `, and must be updated every 3 days during code slush.

Example update:

` + "```" + `
ACK.  In progress
ETA: DD/MM/YYYY
Risks: Complicated fix required
` + "```" + `
<details>` + blockerCompleteLabels,
		},
		"slush - Complete labels, approved, blocker, update not due": {
			mode:             milestoneModeSlush,
			labels:           []string{"status/in-progress"},
			labelsComplete:   true,
			isApproved:       true,
			isBlocker:        true,
			events:           milestoneLabelEvents(statusApprovedLabel, createdNow),
			comments:         milestoneIssueComments(createdNow),
			expectedSections: sets.NewString("summarizeLabels", "warnUpdateInterval"),
			expectedState:    milestoneCurrent,
			expectedBody: `
**Note**: This issue is marked as ` + "`priority/critical-urgent`" + `, and must be updated every 3 days during code slush.

Example update:

` + "```" + `
ACK.  In progress
ETA: DD/MM/YYYY
Risks: Complicated fix required
` + "```" + `
<details open>` + blockerCompleteLabels,
		},
		"slush - Complete labels, approved, blocker, update due": {
			mode:             milestoneModeSlush,
			labels:           []string{"status/in-progress"},
			labelsComplete:   true,
			isApproved:       true,
			isBlocker:        true,
			events:           milestoneLabelEvents(statusApprovedLabel, createdNow),
			comments:         milestoneIssueComments(createdPastSlushUpdateInterval),
			expectedSections: sets.NewString("summarizeLabels", "warnUpdateInterval", "warnUpdateRequired"),
			expectedState:    milestoneNeedsAttention,
			expectedBody: `
**Action Required**: This issue has not been updated since ` + createdPastSlushUpdateInterval.Format("Jan 2") + `. Please provide an update.

**Note**: This issue is marked as ` + "`priority/critical-urgent`" + `, and must be updated every 3 days during code slush.

Example update:

` + "```" + `
ACK.  In progress
ETA: DD/MM/YYYY
Risks: Complicated fix required
` + "```" + `
<details>` + blockerCompleteLabels,
		},
		"freeze - Complete labels, approved, non-blocker": {
			mode:             milestoneModeFreeze,
			labelsComplete:   true,
			isApproved:       true,
			expectedSections: sets.NewString("summarizeLabels", "removeNonBlocker"),
			expectedState:    milestoneNeedsRemoval,
			expectedBody:     "**Important**: Code freeze is in effect and only issues with `priority/critical-urgent` may remain in the v1.8 milestone.",
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			mode := milestoneModeDev
			if len(test.mode) > 0 {
				mode = test.mode
			}
			munger.milestoneModeMap = map[string]string{milestoneString: mode}

			labels := test.labels
			if test.isBlocker {
				labels = append(labels, blockerLabel)
			} else {
				labels = append(labels, "priority/important-soon")
			}
			if test.labelsComplete {
				labels = append(labels, "kind/bug")
				labels = append(labels, "sig/foo")
			}
			if test.isApproved {
				labels = append(labels, statusApprovedLabel)
			}

			issue := github_test.Issue("user", 1, labels, false)
			// Ensure issue was created before any comments or events
			createdLongAgo := createdNow.Add(-28 * day)
			issue.CreatedAt = &createdLongAgo
			milestone := &githubapi.Milestone{Title: stringPtr(milestoneString), Number: intPtr(1)}
			issue.Milestone = milestone

			client, server, mux := github_test.InitServer(t, issue, nil, test.events, nil, nil, nil, nil)
			defer server.Close()

			config := &github.Config{Org: "o", Project: "r"}

			path := fmt.Sprintf("/repos/%s/%s/issues/%d", config.Org, config.Project, *issue.Number)
			mux.HandleFunc(fmt.Sprintf("%s/comments", path), func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.WriteHeader(http.StatusOK)
					data, err := json.Marshal(test.comments)
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					w.Write(data)
					return
				}
				t.Fatalf("Unexpected method: %s", r.Method)
			})

			config.SetClient(client)
			obj, err := config.GetObject(*issue.Number)
			if err != nil {
				t.Fatal(err)
			}

			icc := munger.issueChangeConfig(obj)
			if icc == nil {
				t.Fatalf("%s: Expected non-nil issue change config", testName)
			}

			if !test.expectedSections.Equal(icc.enabledSections) {
				t.Fatalf("%s: Expected sections %v, got %v", testName, test.expectedSections, icc.enabledSections)
			}

			if test.expectedState != icc.state {
				t.Fatalf("%s: Expected state %v, got %v", testName, test.expectedState, icc.state)
			}

			messageBody := icc.messageBody()
			if messageBody == nil {
				t.Fatalf("%s: Expected non-nil message body", testName)
			}
			expectedBody := strings.TrimSpace(test.expectedBody)
			trimmedBody := strings.TrimSpace(*messageBody)
			if expectedBody != trimmedBody {
				t.Fatalf("%s: Expected message body:\n\n%s\nGot:\n\n%s", testName, expectedBody, trimmedBody)
			}
		})
	}
}

func milestoneTestComment(title string, context string, createdAt time.Time) *c.Comment {
	n := &c.Notification{
		Name:      milestoneNotifierName,
		Arguments: title,
		Context:   context,
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

func milestoneIssueComments(createdAt time.Time) []*githubapi.IssueComment {
	return []*githubapi.IssueComment{
		{
			Body:      stringPtr("foo"),
			UpdatedAt: &createdAt,
			CreatedAt: &createdAt,
			User: &githubapi.User{
				Login: githubapi.String("bar"),
			},
		},
	}
}

func TestNotificationIsCurrent(t *testing.T) {
	createdNow := time.Now()
	warningInterval := day
	createdYesterday := createdNow.Add(-(warningInterval + time.Hour))

	realSample := "@foo @bar @baz\n\n**Action required**: This issue requires label changes. If the required changes are not made within 6 days, the issue will be moved out of the v1.8 milestone.\n\n_**kind**_: Must specify at most one of [`kind/bug`, `kind/cleanup`, `kind/feature`].\n_**priority**_: Must specify at most one of [`priority/critical-urgent`, `priority/important-longterm`, `priority/important-soon`].\n_**sig owner**_: Must specify at least one label prefixed with `sig/`.\n\n<details>\nAdditional instructions available <a href=\"https://git.k8s.io/sig-release/ephemera/issues.md\">here</a>\n</details>"

	tests := map[string]struct {
		message            string
		newMessage         string
		createdAt          time.Time
		nilCommentInterval bool
		expectedIsCurrent  bool
	}{
		"Not current if no notification exists": {},
		"Not current if the message is different": {
			message:    "foo",
			newMessage: "bar",
		},
		"Not current if the warning interval has elapsed": {
			message:    "foo",
			newMessage: "foo",
			createdAt:  createdYesterday,
		},
		"Not current if the message is different and the comment interval is nil": {
			message:            "foo",
			newMessage:         "bar",
			nilCommentInterval: true,
		},
		"Notification is current, real sample": {
			message:           realSample,
			newMessage:        realSample,
			createdAt:         createdNow,
			expectedIsCurrent: true,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			var oldComment *c.Comment
			if len(test.message) > 0 {
				oldComment = milestoneTestComment("foo", test.message, test.createdAt)
			}
			newComment := milestoneTestComment("foo", test.newMessage, createdNow)
			notification := c.ParseNotification(newComment)
			var commentInterval *time.Duration
			if !test.nilCommentInterval {
				commentInterval = &warningInterval
			}
			isCurrent := notificationIsCurrent(notification, oldComment, commentInterval)
			if test.expectedIsCurrent != isCurrent {
				t.Logf("notification %#v\n", notification)
				t.Fatalf("%s: expected warningIsCurrent to be %t, but got %t", testName, test.expectedIsCurrent, isCurrent)
			}
		})
	}
}

func TestIgnoreObject(t *testing.T) {
	tests := map[string]struct {
		isClosed        bool
		milestone       string
		activeMilestone string
		expectedIgnore  bool
	}{
		"Ignore closed issue": {
			isClosed:       true,
			expectedIgnore: true,
		},
		"Do not ignore open issue": {},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			issue := github_test.Issue("user", 1, nil, false)
			issue.Milestone = &githubapi.Milestone{Title: stringPtr(test.milestone), Number: intPtr(1)}
			if test.isClosed {
				issue.State = stringPtr("closed")
			}
			obj := &github.MungeObject{Issue: issue}

			ignore := ignoreObject(obj)

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
	if len(expectedLabelNames) != len(labelNames) {
		t.Fatalf("Wrong number of labels. Got %v, wanted %v.", labelNames, expectedLabelNames)
	}
	for _, ln1 := range expectedLabelNames {
		var found bool
		for _, ln2 := range labelNames {
			if ln1 == ln2 {
				found = true
			}
		}
		if !found {
			t.Errorf("Label %s not found in %v", ln1, labelNames)
		}
	}
}

func TestParseMilestoneModes(t *testing.T) {
	tests := map[string]struct {
		input       string
		output      map[string]string
		errExpected bool
	}{
		"Empty string": {
			errExpected: true,
		},
		"Too many = separators": {
			input:       "v1.8==dev",
			errExpected: true,
		},
		"Too many , separators": {
			input:       "v1.8=dev,,v1.9=dev",
			errExpected: true,
		},
		"Missing milestone": {
			input:       "=dev",
			errExpected: true,
		},
		"Missing mode": {
			input:       "v1.8=",
			errExpected: true,
		},
		"Invalid mode": {
			input:       "v1.8=foo",
			errExpected: true,
		},
		"Duplicated milestone": {
			input:       "v1.8=dev,v1.8=slush",
			errExpected: true,
		},
		"Single milestone": {
			input:  "v1.8=dev",
			output: map[string]string{"v1.8": "dev"},
		},
		"Multiple milestones": {
			input:  "v1.8=dev,v1.9=slush",
			output: map[string]string{"v1.8": "dev", "v1.9": "slush"},
		},
	}
	for testName, test := range tests {
		output, err := parseMilestoneModes(test.input)
		if test.errExpected && err == nil {
			t.Fatalf("%s: Expected an error to have occurred", testName)
		}
		if !test.errExpected && err != nil {
			t.Fatalf("%s: Expected no error but got: %v", testName, err)
		}

		if !reflect.DeepEqual(test.output, output) {
			t.Fatalf("%s: Expected output %v, got %v", testName, test.output, output)
		}
	}
}
