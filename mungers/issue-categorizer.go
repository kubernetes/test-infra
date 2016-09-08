/*
Copyright 2015 The Kubernetes Authors.

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
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/matchers/event"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// LabelMunger will update a label on a PR based on how many lines are changed.
// It will exclude certain files in it's calculations based on the config
// file provided in --generated-files-config
type LabelMunger struct {
	TriagerUrl string
}

// Initialize will initialize the munger
func (LabelMunger) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// Name is the name usable in --pr-mungers
func (LabelMunger) Name() string { return "issue-triager" }

// RequiredFeatures is a slice of 'features' that must be provided
func (LabelMunger) RequiredFeatures() []string { return []string{} }

// AddFlags will add any request flags to the cobra `cmd`
func (lm *LabelMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&lm.TriagerUrl, "triager-url", "", "Url on which ml web service is listening")
}

func init() {
	lm := &LabelMunger{}
	RegisterMungerOrDie(lm)
}

// EachLoop is called at the start of every munge loop
func (LabelMunger) EachLoop() error { return nil }

// Munge is the workhorse the will actually make updates to the PR
func (lm *LabelMunger) Munge(obj *github.MungeObject) {
	//this munger only works on issues
	if obj.IsPR() {
		return
	}
	if obj.HasLabel("kind/flake") {
		return
	}

	tLabels := github.GetLabelsWithPrefix(obj.Issue.Labels, "team/")
	cLabels := github.GetLabelsWithPrefix(obj.Issue.Labels, "component/")

	if len(tLabels) == 0 && len(cLabels) == 0 {
		obj.AddLabels(getRoutingLabels(lm.TriagerUrl, obj.Issue.Title, obj.Issue.Body))
	}
	// else {
	// 	newLabels := needsUpdate(obj)
	// 	if len(newLabels) != 0 {
	// 		updateModel(lm.TriagerUrl, obj.Issue.Title, obj.Issue.Body, newLabels)
	// 	}
	// }
}

func updateModel(triagerUrl string, title, body *string, newLabels []string) {
	glog.Infof("Updating the models on the server: %v", triagerUrl)
	_, err := http.PostForm(triagerUrl,
		url.Values{"titles": []string{*title},
			"bodies": []string{*body},
			"labels": newLabels})
	if err != nil {
		glog.Error(err)
	}
}

func needsUpdate(obj *github.MungeObject) []string {
	newLabels := []string{}

	newTeamLabel := getHumanCorrectedLabel(obj, "team")
	if newTeamLabel != nil {
		newLabels = append(newLabels, *newTeamLabel)
	}

	newComponentLabel := getHumanCorrectedLabel(obj, "component")
	if newComponentLabel != nil {
		newLabels = append(newLabels, *newComponentLabel)
	}
	return newLabels
}

func getRoutingLabels(triagerUrl string, title, body *string) []string {
	glog.Infof("Asking the server for labels: %v", triagerUrl)

	if title == nil || body == nil {
		glog.Warning("Title or Body cannot be nil")
		return []string{}
	}
	routingLabelsToApply, err := http.PostForm(triagerUrl,
		url.Values{"title": {*title}, "body": {*body}})

	if err != nil {
		glog.Error(err)
		return []string{}
	}
	defer routingLabelsToApply.Body.Close()
	response, err := ioutil.ReadAll(routingLabelsToApply.Body)
	if err != nil {
		glog.Error(err)
		return []string{}
	}
	if routingLabelsToApply.StatusCode != 200 {
		glog.Errorf("%d: %s", routingLabelsToApply.StatusCode, response)
		return []string{}
	}
	return strings.Split(string(response), ",")
}

func getHumanCorrectedLabel(obj *github.MungeObject, s string) *string {
	myEvents, err := obj.GetEvents()

	if err != nil {
		glog.Errorf("Could not get the events associated with Issue %d", obj.Issue.Number)
		return nil
	}

	botEvents := event.FilterEvents(myEvents, event.And([]event.Matcher{event.BotActor(), event.AddLabel{}, event.LabelPrefix(s)}))

	if botEvents.Empty() {
		return nil
	}

	humanEventsAfter := event.FilterEvents(
		myEvents,
		event.And([]event.Matcher{
			event.HumanActor(),
			event.AddLabel{},
			event.LabelPrefix(s),
			event.CreatedAfter(*botEvents.GetLast().CreatedAt),
		}),
	)

	if humanEventsAfter.Empty() {
		return nil
	}
	lastHumanLabel := humanEventsAfter.GetLast()

	glog.Infof("Recopying human-added label: %s for PR %d", *lastHumanLabel.Label.Name, *obj.Issue.Number)
	obj.RemoveLabel(*lastHumanLabel.Label.Name)
	obj.AddLabel(*lastHumanLabel.Label.Name)
	return lastHumanLabel.Label.Name
}
