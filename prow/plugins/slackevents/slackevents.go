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
	"strings"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
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
	plugins.RegisterPushEventHandler(pluginName, handlePush, helpProvider)
	plugins.RegisterGenericCommentHandler(pluginName, handleComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{
		"": fmt.Sprintf("SIG mentions on Github are reiterated for the following SIG Slack channels: %s.", strings.Join(config.Slack.MentionChannels, ", ")),
	}
	for _, repo := range enabledRepos {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", repo)
		}
		if mw := getMergeWarning(config.Slack.MergeWarnings, parts[0], parts[1]); mw != nil {
			configInfo[repo] = fmt.Sprintf("In this repo merges are considered "+
				"manual and trigger manual merge warnings if the user who merged is not "+
				"a member of this universal whitelist: %s or merged to a branch they "+
				"are not specifically whitelisted for: %#v.<br>Warnings are sent to the "+
				"following Slack channels: %s.", strings.Join(mw.WhiteList, ", "),
				mw.BranchWhiteList, strings.Join(mw.Channels, ", "))
		} else {
			configInfo[repo] = "There are no manual merge warnings configured for this repo."
		}
	}
	return &pluginhelp.PluginHelp{
			Description: `The slackevents plugin reacts to various Github events by commenting in Slack channels.
<ol><li>The plugin can create comments to alert on manual merges. Manual merges are merges made by a normal user instead of a bot or trusted user.</li>
<li>The plugin can create comments to reiterate SIG mentions like '@kubernetes/sig-testing-bugs' from Github.</li></ol>`,
			Config: configInfo,
		},
		nil
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
	//Fetch MergeWarning for the repo we received the merge event.
	if mw := getMergeWarning(pc.SlackConfig.MergeWarnings, pe.Repo.Owner.Login, pe.Repo.Name); mw != nil {
		//If the MergeWarning whitelist has the merge user then no need to send a message.
		if wl := !isWhiteListed(mw, pe); wl {
			var message string
			switch {
			case pe.Created:
				message = fmt.Sprintf("*Warning:* %s (<@%s>) pushed a new branch (%s): %s", pe.Sender.Login, pe.Sender.Login, pe.Branch(), pe.Compare)
			case pe.Deleted:
				message = fmt.Sprintf("*Warning:* %s (<@%s>) deleted a branch (%s): %s", pe.Sender.Login, pe.Sender.Login, pe.Branch(), pe.Compare)
			case pe.Forced:
				message = fmt.Sprintf("*Warning:* %s (<@%s>) *force* merged %d commit(s) into %s: %s", pe.Sender.Login, pe.Sender.Login, len(pe.Commits), pe.Branch(), pe.Compare)
			default:
				message = fmt.Sprintf("*Warning:* %s (<@%s>) manually merged %d commit(s) into %s: %s", pe.Sender.Login, pe.Sender.Login, len(pe.Commits), pe.Branch(), pe.Compare)
			}
			for _, channel := range mw.Channels {
				if err := pc.SlackClient.WriteMessage(message, channel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isWhiteListed(mw *plugins.MergeWarning, pe github.PushEvent) bool {
	bwl := mw.BranchWhiteList[pe.Branch()]
	inWhiteList := stringInArray(pe.Pusher.Name, mw.WhiteList) || stringInArray(pe.Sender.Login, mw.WhiteList)
	inBranchWhiteList := stringInArray(pe.Pusher.Name, bwl) || stringInArray(pe.Sender.Login, bwl)
	return inWhiteList || inBranchWhiteList
}

func getMergeWarning(mergeWarnings []plugins.MergeWarning, org, repo string) *plugins.MergeWarning {
	for _, mw := range mergeWarnings {
		if stringInArray(org, mw.Repos) || stringInArray(fmt.Sprintf("%s/%s", org, repo), mw.Repos) {
			return &mw
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

		msg := fmt.Sprintf("%s was mentioned by %s (<@%s>) on Github. (%s)\n>>>%s", sig, e.User.Login, e.User.Login, e.HTMLURL, e.Body)
		if err := pc.SlackClient.WriteMessage(msg, sig); err != nil {
			return fmt.Errorf("Failed to send message on slack channel: %q with message %q. Err: %v", sig, msg, err)
		}
	}
	return nil
}
