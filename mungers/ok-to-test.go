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

package mungers

import (
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// OkToTestMunger looks for situations where a reviewer has LGTM'd a PR, but it
// isn't ok to test by the k8s-bot, and adds an 'ok to test' comment to the PR.
type OkToTestMunger struct{}

func init() {
	RegisterMungerOrDie(OkToTestMunger{})
}

// Name is the name usable in --pr-mungers
func (OkToTestMunger) Name() string { return "ok-to-test" }

// Initialize will initialize the munger
func (OkToTestMunger) Initialize(config *github.Config) error { return nil }

// EachLoop is called at the start of every munge loop
func (OkToTestMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (OkToTestMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (OkToTestMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel("lgtm") {
		return
	}
	state := obj.GetStatusState([]string{jenkinsE2EContext, jenkinsUnitContext})
	if state == "incomplete" {
		glog.V(2).Infof("status is incomplete, adding ok to test")
		msg := `@k8s-bot ok to test
@k8s-bot test this

pr builder appears to be missing, activating due to 'lgtm' label.`
		obj.WriteComment(msg)
	}
}
