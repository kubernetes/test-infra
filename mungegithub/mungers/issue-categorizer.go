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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
)

// LabelMunger will update a label on a PR based on how many lines are changed.
// It will exclude certain files in it's calculations based on the config
// file provided in --generated-files-config
type LabelMunger struct {
	triagerUrl string
}

// Initialize will initialize the munger
func (*LabelMunger) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// Name is the name usable in --pr-mungers
func (*LabelMunger) Name() string { return "issue-triager" }

// RequiredFeatures is a slice of 'features' that must be provided
func (*LabelMunger) RequiredFeatures() []string { return []string{} }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (lm *LabelMunger) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&lm.triagerUrl, "triager-url", "", "Url on which ml web service is listening")
	return nil
}

func init() {
	RegisterMungerOrDie(&LabelMunger{})
}

// EachLoop is called at the start of every munge loop
func (*LabelMunger) EachLoop() error { return nil }

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
		obj.AddLabels(getRoutingLabels(lm.triagerUrl, obj.Issue.Title, obj.Issue.Body))
	}
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
