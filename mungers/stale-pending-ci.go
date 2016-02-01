/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"fmt"
	"time"

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	stalePendingCIHours = 24
)

// StalePendingCI will ask the k8s-bot to test any PR with a LGTM that has
// been pending for more than 24 hours. This can happen when the jenkins VM
// is restarted.
//
// The real fix would be for the jenkins VM restart to not move every single
// PR to pending without actually testing...
//
// But this is our world and so we should really do this for all PRs which
// aren't likely to get another push (everything that is mergeable). Since that
// can be a lot of PRs, I'm just doing it for the LGTM PRs automatically...
//
// With minor modification this can be run easily by hand. Remove the LGTM check
// godep go build
// ./mungegithub --token-file=/PATH/TO/YOUR/TOKEN --pr-mungers=stale-pending-ci --once (--dry-run)
type StalePendingCI struct{}

func init() {
	RegisterMungerOrDie(StalePendingCI{})
}

// Name is the name usable in --pr-mungers
func (StalePendingCI) Name() string { return "stale-pending-ci" }

// Initialize will initialize the munger
func (StalePendingCI) Initialize(config *github.Config) error { return nil }

// EachLoop is called at the start of every munge loop
func (StalePendingCI) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (StalePendingCI) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (StalePendingCI) Munge(obj *github.MungeObject) {
	requiredContexts := []string{jenkinsUnitContext, jenkinsE2EContext}

	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel("lgtm") {
		return
	}

	if mergeable, err := obj.IsMergeable(); !mergeable || err != nil {
		return
	}

	status := obj.GetStatusState(requiredContexts)
	if status != "pending" {
		return
	}

	for _, context := range requiredContexts {
		statusTime := obj.GetStatusTime(context)
		if statusTime == nil {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > stalePendingCIHours*time.Hour {
			msgFormat := `@k8s-bot test this issue: #IGNORE

Tests have been pending for %d hours`
			msg := fmt.Sprintf(msgFormat, stalePendingCIHours)
			obj.WriteComment(msg)
			return
		}
	}
}
