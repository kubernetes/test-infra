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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/slack"
)

type FakeClient struct {
	SentMessages map[string][]string
}

func (fk *FakeClient) WriteMessage(text string, channel string) error {
	fk.SentMessages[channel] = append(fk.SentMessages[channel], text)
	return nil
}

func TestPush(t *testing.T) {
	var pushStr = `{
  "ref": "refs/heads/master",
  "before": "d73a75b4b1ddb63870954b9a60a63acaa4cb1ca5",
  "after": "045a6dca07840efaf3311450b615e19b5c75f787",
  "created": false,
  "deleted": false,
  "forced": false,
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
	pushEvManual.Ref = "refs/heads/master"

	pushEvManualBranchExempted := pushEv
	pushEvManualBranchExempted.Pusher.Name = "Warren Teened"
	pushEvManualBranchExempted.Pusher.Email = "wteened@users.noreply.github.com"
	pushEvManualBranchExempted.Sender.Login = "WTeened"
	pushEvManualBranchExempted.Ref = "refs/heads/warrens-branch"

	pushEvManualNotBranchExempted := pushEvManualBranchExempted
	pushEvManualNotBranchExempted.Ref = "refs/heads/master"

	pushEvManualCreated := pushEvManual
	pushEvManualCreated.Created = true
	pushEvManualCreated.Ref = "refs/heads/release-1.99"
	pushEvManualCreated.Compare = "https://github.com/kubernetes/kubernetes/compare/045a6dca0784"

	pushEvManualDeleted := pushEvManual
	pushEvManualDeleted.Deleted = true
	pushEvManualDeleted.Ref = "refs/heads/release-1.99"
	pushEvManualDeleted.Compare = "https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...000000000000"

	pushEvManualForced := pushEvManual
	pushEvManualForced.Forced = true

	noMessages := map[string][]string{}
	stdWarningMessages := map[string][]string{
		"sig-contribex":  {"*Warning:* tester (<@tester>) manually merged 2 commit(s) into master: https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"},
		"kubernetes-dev": {"*Warning:* tester (<@tester>) manually merged 2 commit(s) into master: https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"}}

	createdWarningMessages := map[string][]string{
		"sig-contribex":  {"*Warning:* tester (<@tester>) pushed a new branch (release-1.99): https://github.com/kubernetes/kubernetes/compare/045a6dca0784"},
		"kubernetes-dev": {"*Warning:* tester (<@tester>) pushed a new branch (release-1.99): https://github.com/kubernetes/kubernetes/compare/045a6dca0784"}}

	deletedWarningMessages := map[string][]string{
		"sig-contribex":  {"*Warning:* tester (<@tester>) deleted a branch (release-1.99): https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...000000000000"},
		"kubernetes-dev": {"*Warning:* tester (<@tester>) deleted a branch (release-1.99): https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...000000000000"}}

	forcedWarningMessages := map[string][]string{
		"sig-contribex":  {"*Warning:* tester (<@tester>) *force* merged 2 commit(s) into master: https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"},
		"kubernetes-dev": {"*Warning:* tester (<@tester>) *force* merged 2 commit(s) into master: https://github.com/kubernetes/kubernetes/compare/d73a75b4b1dd...045a6dca0784"}}

	type testCase struct {
		name             string
		pushReq          github.PushEvent
		expectedMessages map[string][]string
	}

	testcases := []testCase{
		{
			name:             "If PR merged manually by a user, we send message to sig-contribex and kubernetes-dev.",
			pushReq:          pushEvManual,
			expectedMessages: stdWarningMessages,
		},
		{
			name:             "If PR force merged by a user, we send message to sig-contribex and kubernetes-dev with force merge message.",
			pushReq:          pushEvManualForced,
			expectedMessages: forcedWarningMessages,
		},
		{
			name:             "If PR merged by k8s merge bot we should NOT send message to sig-contribex and kubernetes-dev.",
			pushReq:          pushEv,
			expectedMessages: noMessages,
		},
		{
			name:             "If PR merged by a user not in the exemption list but in THIS branch exemption list, we should NOT send a message to sig-contribex and kubernetes-dev.",
			pushReq:          pushEvManualBranchExempted,
			expectedMessages: noMessages,
		},
		{
			name:             "If PR merged by a user not in the exemption list, in a branch exemption list, but not THIS branch exemption list, we should send a message to sig-contribex and kubernetes-dev.",
			pushReq:          pushEvManualBranchExempted,
			expectedMessages: noMessages,
		},
		{
			name:             "If a branch is created by a non-exempted user, we send message to sig-contribex and kubernetes-dev with branch created message.",
			pushReq:          pushEvManualCreated,
			expectedMessages: createdWarningMessages,
		},
		{
			name:             "If a branch is deleted by a non-exempted user, we send message to sig-contribex and kubernetes-dev with branch deleted message.",
			pushReq:          pushEvManualDeleted,
			expectedMessages: deletedWarningMessages,
		},
	}

	pc := client{
		SlackConfig: plugins.Slack{
			MergeWarnings: []plugins.MergeWarning{
				{
					Repos:       []string{"kubernetes/kubernetes"},
					Channels:    []string{"kubernetes-dev", "sig-contribex"},
					ExemptUsers: []string{"k8s-merge-robot"},
					ExemptBranches: map[string][]string{
						"warrens-branch": {"wteened"},
					},
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
		slackClient := &FakeClient{
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
			name:             "If multiple sigs mentioned, but only one channel is allowed, only send to one channel.",
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
		fakeSlackClient := &FakeClient{
			SentMessages: make(map[string][]string),
		}
		client := client{
			GitHubClient: &fakegithub.FakeClient{},
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

func TestHelpProvider(t *testing.T) {
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	cases := []struct {
		name         string
		config       *plugins.Configuration
		enabledRepos []config.OrgRepo
		err          bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: enabledRepos,
		},
		{
			name: "All configs enabled",
			config: &plugins.Configuration{
				Slack: plugins.Slack{
					MentionChannels: []string{"chan1", "chan2"},
					MergeWarnings: []plugins.MergeWarning{
						{
							Repos:       []string{"org2/repo"},
							Channels:    []string{"chan1", "chan2"},
							ExemptUsers: []string{"k8s-merge-robot"},
							ExemptBranches: map[string][]string{
								"warrens-branch": {"wteened"},
							},
						},
					},
				},
			},
			enabledRepos: enabledRepos,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
		})
	}
}
