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
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// LGTMAfterCommitMunger will remove the LGTM flag from an PR which has been
// updated since the reviewer added LGTM
type LGTMAfterCommitMunger struct{}

func init() {
	RegisterMungerOrDie(LGTMAfterCommitMunger{})
}

// Name is the name usable in --pr-mungers
func (LGTMAfterCommitMunger) Name() string { return "lgtm-after-commit" }

// RequiredFeatures is a slice of 'features' that must be provided
func (LGTMAfterCommitMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (LGTMAfterCommitMunger) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (LGTMAfterCommitMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (LGTMAfterCommitMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (LGTMAfterCommitMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel("lgtm") {
		return
	}

	lastModified := obj.LastModifiedTime()
	lgtmTime := obj.LabelTime("lgtm")

	if lastModified == nil || lgtmTime == nil {
		glog.Errorf("PR %d unable to determine lastModified or lgtmTime", *obj.Issue.Number)
		return
	}

	if lastModified.After(*lgtmTime) {
		glog.Infof("PR: %d lgtm:%s  lastModified:%s", *obj.Issue.Number, lgtmTime.String(), lastModified.String())
		lgtmRemovedBody := "PR changed after LGTM, removing LGTM."
		if err := obj.WriteComment(lgtmRemovedBody); err != nil {
			return
		}
		obj.RemoveLabel("lgtm")
	}
}
