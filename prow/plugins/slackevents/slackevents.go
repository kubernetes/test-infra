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

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "slackevents"
)

type slackClient interface {
	WriteMessage(text string, channel string) error
}

type client struct {
	SlackClient slackClient
	SlackEvents []plugins.SlackEvent
}

func init() {
	plugins.RegisterPushEventHandler(pluginName, handlePush)
}

func handlePush(pc plugins.PluginClient, pe github.PushEvent) error {
	c := client{
		SlackEvents: pc.PluginConfig.SlackEvents,
		SlackClient: pc.SlackClient,
	}
	return notifyOnSlackIfManualMerge(c, pe)
}

func notifyOnSlackIfManualMerge(pc client, pe github.PushEvent) error {
	//Fetch slackevent configuration for the repo we received the merge event.
	if se := getSlackEvent(pc, pe.Repo.Owner.Login, pe.Repo.Name); se != nil {
		//If the slackevent whitelist has the merge user then no need to send a message.
		if !stringInArray(pe.Pusher.Name, se.WhiteList) && !stringInArray(pe.Sender.Login, se.WhiteList) {
			message := fmt.Sprintf("Warning: @%s manually merged %s", pe.Sender.Login, pe.Compare)
			for _, channel := range se.Channels {
				if err := pc.SlackClient.WriteMessage(message, channel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func getSlackEvent(pc client, org, repo string) *plugins.SlackEvent {
	for _, se := range pc.SlackEvents {
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
