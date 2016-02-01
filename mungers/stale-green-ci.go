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
	"fmt"
	"time"

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	staleGreenCIHours = 48
)

// StaleGreenCI will remove the LGTM flag from an PR which has been
// updated since the reviewer added LGTM
type StaleGreenCI struct{}

func init() {
	RegisterMungerOrDie(StaleGreenCI{})
}

// Name is the name usable in --pr-mungers
func (StaleGreenCI) Name() string { return "stale-green-ci" }

// Initialize will initialize the munger
func (StaleGreenCI) Initialize(config *github.Config) error { return nil }

// EachLoop is called at the start of every munge loop
func (StaleGreenCI) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (StaleGreenCI) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (StaleGreenCI) Munge(obj *github.MungeObject) {
	requiredContexts := []string{jenkinsUnitContext, jenkinsE2EContext}

	if !obj.IsPR() {
		return
	}

	if !obj.HasLabels([]string{"lgtm"}) {
		return
	}

	if mergeable, err := obj.IsMergeable(); !mergeable || err != nil {
		return
	}

	if !obj.IsStatusSuccess(requiredContexts) {
		return
	}

	for _, context := range requiredContexts {
		statusTime := obj.GetStatusTime(context)
		if statusTime == nil {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > staleGreenCIHours*time.Hour {
			msgFormat := `@k8s-bot test this

Tests are more than %d hours old. Re-running tests.`
			msg := fmt.Sprintf(msgFormat, staleGreenCIHours)
			obj.WriteComment(msg)
			err := obj.WaitForPending(requiredContexts)
			if err != nil {
				glog.Errorf("Failed waiting for PR to start testing: %v", err)
			}
			return
		}
	}
}
