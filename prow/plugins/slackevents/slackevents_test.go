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

package slackevents

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/slack"
	"k8s.io/test-infra/prow/slack/fakeslack"
)

func TestPush(t *testing.T) {
	var pushStr string = `{
  "ref": "refs/heads/master",
  "before": "d73a75b4b1ddb63870954b9a60a63acaa4cb1ca5",
  "after": "045a6dca07840efaf3311450b615e19b5c75f787",
  "compare": "https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784",
  "commits": [
    {
      "id": "8427d5a27478c80167fd66affe1bd7cd01d3f9a8",
      "message": "Decrease fluentd cpu request",
      "url": "https://github.com/kubernetes/kubernetes/commit/8427d5a27478c80167fd66affe1bd7cd01d3f9a8"
    },
    {
      "id": "045a6dca07840efaf3311450b615e19b5c75f787",
      "message": "Merge pull request #47906 from gmarek/fluentd\n\nDecrese fluentd cpu request\n\nFix #47905\r\n\r\ncc @piosz - this should fix your tests.\r\ncc @dchen1107",
      "url": "https://github.com/kubernetes/kubernetes/commit/045a6dca07840efaf3311450b615e19b5c75f787"
    }
  ],
  "repository": {
    "id": 20580498,
    "name": "kubernetes",
    "owner": {
	"name": "kubernetes",
	"login": "kubernetes"
    },
    "url": "https://github.com/kubernetes/kubernetes"
  },
  "pusher": {
    "name": "k8s-merge-robot",
    "email": "k8s-merge-robot@users.noreply.github.com"
  }
}`

	var pushEv github.PushEvent
	if err := json.Unmarshal([]byte(pushStr), &pushEv); err != nil {
		t.Fatalf("Failed to parse Push Notification: %s", err)
	}

	// Non bot user merged the PR
	pushEvManual := pushEv
	pushEvManual.Pusher.Name = "Jester Tester"
	pushEvManual.Pusher.Email = "tester@users.noreply.github.com"
	pushEvManual.Sender.Login = "tester"

	type testCase struct {
		name             string
		pushReq          github.PushEvent
		expectedMessages map[string][]string
	}

	testcases := []testCase{
		{
			name:    "If PR merged manually by a user we send message to sig-contribex and kubernetes-dev.",
			pushReq: pushEvManual,
			expectedMessages: map[string][]string{
				"sig-contribex":  {"*Warning:* tester (<@tester>) manually merged https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"},
				"kubernetes-dev": {"*Warning:* tester (<@tester>) manually merged https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"}},
		},
		{
			name:             "If PR merged by k8s merge bot we should NOT send message to sig-contribex and kubernetes-dev.",
			pushReq:          pushEv,
			expectedMessages: map[string][]string{},
		},
	}

	pc := client{
		SlackConfig: plugins.Slack{
			MergeWarnings: []plugins.MergeWarning{
				{
					Repos:     []string{"kubernetes/kubernetes"},
					Channels:  []string{"kubernetes-dev", "sig-contribex"},
					WhiteList: []string{"k8s-merge-robot"},
				},
			},
		},
		SlackClient: slack.NewFakeClient(),
	}

	//should not fail if slackClient is nil
	for _, tc := range testcases {
		if err := notifyOnSlackIfManualMerge(pc, tc.pushReq); err != nil {
			t.Fatalf("Didn't expect error if slack client is nil: %s", err)
		}
	}

	//repeat the tests with a fake slack client
	for _, tc := range testcases {
		slackClient := &fakeslack.FakeClient{
			SentMessages: make(map[string][]string),
		}
		pc.SlackClient = slackClient

		if err := notifyOnSlackIfManualMerge(pc, tc.pushReq); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		if len(tc.expectedMessages) != len(slackClient.SentMessages) {
			t.Fatalf("Test: %s The number of messages sent do not tally. Expecting %d messages but received %d messages.",
				tc.name, len(tc.expectedMessages), len(slackClient.SentMessages))
		}
		for k, v := range tc.expectedMessages {
			if _, ok := slackClient.SentMessages[k]; !ok {
				t.Fatalf("Test: %s Messages is not sent to channel %s", tc.name, k)
			}
			if strings.Compare(v[0], slackClient.SentMessages[k][0]) != 0 {
				t.Fatalf("Expecting message: %s\nReceived message: %s", v, slackClient.SentMessages[k])
			}
			if len(v) != len(slackClient.SentMessages[k]) {
				t.Fatalf("Test: %s All messages are not delivered to the channel ", tc.name)
			}
		}
	}

}

//Make sure we are sending message to proper sig mentions
func TestComment(t *testing.T) {
	orgMember := "cjwagner"
	bot := "k8s-ci-robot"
	type testCase struct {
		name             string
		action           github.GenericCommentEventAction
		body             string
		expectedMessages map[string][]string
		issueLabels      []string
		repoLabels       []string
		commenter        string
	}
	testcases := []testCase{
		{
			name:             "If sig mentioned then we send a message to the sig with the body of the comment",
			action:           github.GenericCommentActionCreated,
			body:             "@kubernetes/sig-node-misc This issue needs update.",
			expectedMessages: map[string][]string{"sig-node": {"This issue needs update."}},
			commenter:        orgMember,
		},
		{
			name:             "Don't sent message if comment isn't new.",
			action:           github.GenericCommentActionEdited,
			body:             "@kubernetes/sig-node-misc This issue needs update.",
			expectedMessages: map[string][]string{},
			commenter:        orgMember,
		},
		{
			name:             "Don't sent message if commenter is the bot.",
			action:           github.GenericCommentActionEdited,
			body:             "@kubernetes/sig-node-misc This issue needs update.",
			expectedMessages: map[string][]string{},
			commenter:        bot,
		},
		{
			name:             "If multiple sigs mentioned, we send a message to each sig with the body of the comment",
			action:           github.GenericCommentActionCreated,
			body:             "@kubernetes/sig-node-misc, @kubernetes/sig-api-machinery-misc Message sent to multiple sigs.",
			expectedMessages: map[string][]string{"sig-api-machinery": {"Message sent to multiple sigs."}, "sig-node": {"Message sent to multiple sigs."}},
			commenter:        orgMember,
		},
		{
			name:             "If multiple sigs mentioned, but only one channel is whitelisted, only send to one channel.",
			action:           github.GenericCommentActionCreated,
			body:             "@kubernetes/sig-node-misc, @kubernetes/sig-testing-misc Message sent to multiple sigs.",
			expectedMessages: map[string][]string{"sig-node": {"Message sent to multiple sigs."}},
			issueLabels:      []string{},
			commenter:        orgMember,
		},
		{
			name:             "Message should not be sent if the pattern for the channel does not match",
			action:           github.GenericCommentActionCreated,
			body:             "@kubernetes/node-misc No message sent",
			expectedMessages: map[string][]string{},
			commenter:        orgMember,
		},
		{
			name:             "Message sent only if the pattern for the channel match",
			action:           github.GenericCommentActionCreated,
			body:             "@kubernetes/node-misc @kubernetes/sig-api-machinery-bugs Message sent to matching sigs.",
			expectedMessages: map[string][]string{"sig-api-machinery": {"Message sent to matching sigs."}},
			commenter:        orgMember,
		},
	}

	for _, tc := range testcases {
		fakeSlackClient := &fakeslack.FakeClient{
			SentMessages: make(map[string][]string),
		}
		client := client{
			GithubClient: &fakegithub.FakeClient{},
			SlackClient:  fakeSlackClient,
			SlackConfig:  plugins.Slack{MentionChannels: []string{"sig-node", "sig-api-machinery"}},
		}
		e := github.GenericCommentEvent{
			Action: tc.action,
			Body:   tc.body,
			User:   github.User{Login: tc.commenter},
		}

		if err := echoToSlack(client, e); err != nil {
			t.Fatalf("For case %s, didn't expect error from label test: %v", tc.name, err)
		}
		if len(tc.expectedMessages) != len(fakeSlackClient.SentMessages) {
			t.Fatalf("The number of messages sent do not tally. Expecting %d messages but received %d messages.",
				len(tc.expectedMessages), len(fakeSlackClient.SentMessages))
		}
		for k, v := range tc.expectedMessages {
			if _, ok := fakeSlackClient.SentMessages[k]; !ok {
				t.Fatalf("Messages is not sent to channel %s", k)
			}
			if len(v) != len(fakeSlackClient.SentMessages[k]) {
				t.Fatalf("All messages are not delivered to the channel %s", k)
			}
		}
	}
}
