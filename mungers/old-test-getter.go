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
	"strconv"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/e2e"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// OldTestGetter files issues for flaky tests.
type OldTestGetter struct {
	// Keep track of which jobs we've done this for.
	ran                   map[string]bool
	numberOfOldTestsToGet int
	sq                    *SubmitQueue
}

func init() {
	RegisterMungerOrDie(&OldTestGetter{})
}

// Name is the name usable in --pr-mungers
func (p *OldTestGetter) Name() string { return "old-test-getter" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *OldTestGetter) RequiredFeatures() []string { return nil }

// Initialize will initialize the munger
func (p *OldTestGetter) Initialize(config *github.Config, features *features.Features) error {
	// TODO: don't get the mungers from the global list, they should be passed in...
	for _, m := range GetAllMungers() {
		if m.Name() == "submit-queue" {
			p.sq = m.(*SubmitQueue)
			break
		}
	}
	if p.sq == nil {
		return fmt.Errorf("submit-queue not found")
	}
	p.ran = map[string]bool{}
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *OldTestGetter) EachLoop() error {
	if p.sq == nil {
		return fmt.Errorf("submit-queue not found")
	}
	e2eTester, ok := p.sq.e2e.(*e2e.RealE2ETester)
	if !ok {
		return fmt.Errorf("Need real e2e tester, not fake")
	}
	for job, status := range e2eTester.GetBuildStatus() {
		if p.ran[job] {
			continue
		}
		lastRunNumber, err := strconv.Atoi(status.ID)
		if lastRunNumber == 0 || err != nil {
			continue
		}
		for i := 1; i <= p.numberOfOldTestsToGet && i < lastRunNumber; i++ {
			n := lastRunNumber - i
			glog.Infof("Getting results for past test result: %v %v", job, n)
			if _, err := e2eTester.GetBuildResult(job, n); err != nil {
				glog.Errorf("Couldn't get result for %v %v: %v", job, n, err)
			}
		}
		p.ran[job] = true
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *OldTestGetter) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().IntVar(&p.numberOfOldTestsToGet, "number-of-old-test-results", 5, "The number of old test results to get (and therefore file issues for). In case submit queue has some downtime, set this to a higher number and it will file issues for older test runs.")
}

// Munge is unused by this munger.
func (p *OldTestGetter) Munge(obj *github.MungeObject) {}
