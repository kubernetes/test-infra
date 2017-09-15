/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"regexp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "slackevents"
)

var sigMatcher = regexp.MustCompile(`(?m)@kubernetes/sig-([\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)`)

type slackClient interface {
	WriteMessage(text string, channel string) error
}

type githubClient interface {
	BotName() (string, error)
}

type client struct {
	GithubClient githubClient
	SlackClient  slackClient
	SlackConfig  plugins.Slack
}

func init() {
	plugins.RegisterPushEventHandler(pluginName, handlePush)
	plugins.RegisterGenericCommentHandler(pluginName, handleComment)
}

func handleComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	c := client{
		GithubClient: pc.GitHubClient,
		SlackConfig:  pc.PluginConfig.Slack,
		SlackClient:  pc.SlackClient,
	}
	return echoToSlack(c, e)
}

func handlePush(pc plugins.PluginClient, pe github.PushEvent) error {
	c := client{
		GithubClient: pc.GitHubClient,
		SlackConfig:  pc.PluginConfig.Slack,
		SlackClient:  pc.SlackClient,
	}
	return notifyOnSlackIfManualMerge(c, pe)
}

func notifyOnSlackIfManualMerge(pc client, pe github.PushEvent) error {
	//Fetch slackevent configuration for the repo we received the merge event.
	if se := getSlackEvent(pc.SlackConfig.MergeWarnings, pe.Repo.Owner.Login, pe.Repo.Name); se != nil {
		//If the slackevent whitelist has the merge user then no need to send a message.
		if !stringInArray(pe.Pusher.Name, se.WhiteList) && !stringInArray(pe.Sender.Login, se.WhiteList) {
			message := fmt.Sprintf("Warning: <@%s> manually merged %s", pe.Sender.Login, pe.Compare)
			for _, channel := range se.Channels {
				if err := pc.SlackClient.WriteMessage(message, channel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func getSlackEvent(slackEvents []plugins.MergeWarning, org, repo string) *plugins.MergeWarning {
	for _, se := range slackEvents {
		if stringInArray(org, se.Repos) || stringInArray(fmt.Sprintf("%s/%s", org, repo), se.Repos) {
			return &se
		}
	}
	return nil
}

func stringInArray(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func echoToSlack(pc client, e github.GenericCommentEvent) error {
	// Ignore bot comments and comments that aren't new.
	botName, err := pc.GithubClient.BotName()
	if err != nil {
		return err
	}
	if e.User.Login == botName {
		return nil
	}
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	sigMatches := sigMatcher.FindAllStringSubmatch(e.Body, -1)

	for _, match := range sigMatches {
		sig := "sig-" + match[1]
		// Check if this sig is a slack channel that should be messaged.
		found := false
		for _, channel := range pc.SlackConfig.MentionChannels {
			if channel == sig {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		msg := fmt.Sprintf("%s was mentioned by <@%s> on Github. (%s)\n>>>%s", sig, e.User.Login, e.HTMLURL, e.Body)
		if err := pc.SlackClient.WriteMessage(msg, sig); err != nil {
			return fmt.Errorf("Failed to send message on slack channel: %q with message %q. Err: %v", sig, msg, err)
		}
	}
	return nil
}
