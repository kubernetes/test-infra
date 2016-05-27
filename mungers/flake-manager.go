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
	"strings"
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/test-utils/utils"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	// URLTestStorageBucket is a link that works for *directories* and not
	// files, since we want to link people to something that lets them
	// browse all the artifacts.
	URLTestStorageBucket = "https://console.cloud.google.com/storage/kubernetes-jenkins/logs"
)

// issueFinder finds an issue for a given key.
type issueFinder interface {
	AllIssuesForKey(key string) []int
	Created(key string, number int)
	Synced() bool
}

// FlakeManager files issues for flaky tests.
type FlakeManager struct {
	finder               issueFinder
	sq                   *SubmitQueue
	config               *github.Config
	googleGCSBucketUtils *utils.Utils

	oldestTime time.Time

	// map of flake to the issue number
	alreadySyncedFlakes map[cache.Flake]int

	// TODO: go backwards fetching statuses until we reach oldestTime
	// The next run number we need to check for each job
	// jobToRunNumber map[string]int
}

func init() {
	RegisterMungerOrDie(&FlakeManager{})
}

// Name is the name usable in --pr-mungers
func (p *FlakeManager) Name() string { return "flake-manager" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *FlakeManager) RequiredFeatures() []string { return nil }

// Initialize will initialize the munger
func (p *FlakeManager) Initialize(config *github.Config, features *features.Features) error {
	// TODO: don't get the mungers from the global list, they should be passed in...
	for _, m := range GetAllMungers() {
		if m.Name() == "issue-cacher" {
			p.finder = m.(*IssueCacher)
		}
		if m.Name() == "submit-queue" {
			p.sq = m.(*SubmitQueue)
		}
	}
	if p.finder == nil {
		return fmt.Errorf("issue-cacher not found")
	}
	if p.sq == nil {
		return fmt.Errorf("submit-queue not found")
	}
	p.oldestTime = time.Now().Add(-time.Hour * 24)
	p.alreadySyncedFlakes = map[cache.Flake]int{}
	p.config = config
	p.googleGCSBucketUtils = utils.NewUtils(URLTestStorageBucket)
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *FlakeManager) EachLoop() error {
	if p.sq.e2e == nil {
		return fmt.Errorf("submit queue not initialized yet")
	}
	if !p.finder.Synced() {
		return nil
	}
	p.sq.e2e.GCSBasedStable()
	for _, f := range p.sq.e2e.Flakes() {
		p.syncFlake(f)
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *FlakeManager) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is unused by this munger.
func (p *FlakeManager) Munge(obj *github.MungeObject) {}

// Look through all issues filed about this test.
// If foundIn is > 0, then the particular flake was found in that issue.
// All open issues for this test are returned in updatableIssues.
func (p *FlakeManager) findPreviousIssues(f cache.Flake) (foundIn int, updatableIssues []*github.MungeObject, err error) {
	possibleIssues := p.finder.AllIssuesForKey(string(f.Test))
	for _, previousIssue := range possibleIssues {
		obj, err := p.config.GetObject(previousIssue)
		if err != nil {
			return 0, nil, fmt.Errorf("error getting object for %v: %v", previousIssue, err)
		}
		isRecorded, err := p.isRecorded(obj, f)
		if err != nil {
			return 0, nil, fmt.Errorf("error checking whether flake %v is recorded in issue %v: %v", f, previousIssue, err)
		}
		if isRecorded {
			foundIn = previousIssue
			// keep going since we may want to close dups
		}
		if obj.Issue.State != nil && *obj.Issue.State == "open" {
			updatableIssues = append(updatableIssues, obj)
		}
	}
	return foundIn, updatableIssues, nil
}

// Close all of the dups.
func (p *FlakeManager) markAsDups(dups []*github.MungeObject, of int) error {
	// Somehow we got duplicate issues all open at once.
	// Close all of the older ones.
	for _, dup := range dups {
		if err := dup.CloseIssuef("This is a duplicate of #%v; closing", of); err != nil {
			return fmt.Errorf("failed to close %v as a dup of %v: %v", *dup.Issue.Number, of, err)
		}
	}
	return nil
}

func (p *FlakeManager) syncFlake(f cache.Flake) error {
	if _, ok := p.alreadySyncedFlakes[f]; ok {
		return nil
	}

	foundIn, updatableIssues, err := p.findPreviousIssues(f)
	if err != nil {
		return err
	}

	// Close dups if there are multiple open issues
	if len(updatableIssues) > 1 {
		obj := updatableIssues[0]
		if err := p.markAsDups(updatableIssues[1:], *obj.Issue.Number); err != nil {
			return err
		}
	}

	if foundIn > 0 {
		// Don't need to update, we were only here to close the dups.
		p.alreadySyncedFlakes[f] = foundIn
		return nil
	}

	// Update an issue if possible.
	if len(updatableIssues) > 0 {
		obj := updatableIssues[0]
		// Update the chosen issue
		if err := p.updateIssue(obj, f); err != nil {
			return fmt.Errorf("error updating issue %v for %v: %v", *obj.Issue.Number, f, err)
		}
		p.alreadySyncedFlakes[f] = *obj.Issue.Number
		return nil
	}

	// No issue could be updated, create a new issue.
	n, err := p.createIssue(f)
	if err != nil {
		return fmt.Errorf("error making issue for %v: %v", f, err)
	}
	p.finder.Created(string(f.Test), n)
	p.alreadySyncedFlakes[f] = n
	return nil
}

// DO NOT CHANGE or it will not recognize previous entries!
func (p *FlakeManager) flakeID(flake cache.Flake) string {
	return p.googleGCSBucketUtils.GetPathToJenkinsGoogleBucket(string(flake.Job), int(flake.Number), "") + "\n"
}

func (p *FlakeManager) flakeExtraInfo(flake cache.Flake) string {
	return fmt.Sprintf("Failed: %v\n\n```\n%v\n```\n\n", flake.Test, flake.Reason)
}

// Search through the body and comments to see if the given flake is already
// mentioned in the given github issue.
func (p *FlakeManager) isRecorded(obj *github.MungeObject, flake cache.Flake) (bool, error) {
	flakeID := p.flakeID(flake)
	if obj.Issue.Body != nil && strings.Contains(*obj.Issue.Body, flakeID) {
		// We already wrote this flake
		return true, nil
	}
	comments, err := obj.ListComments()
	if err != nil {
		return false, fmt.Errorf("error getting comments for %v: %v", *obj.Issue.Number, err)
	}
	for _, c := range comments {
		if c.Body == nil {
			continue
		}
		if strings.Contains(*c.Body, flakeID) {
			// We already wrote this flake
			return true, nil
		}
	}
	return false, nil
}

// updateIssue adds a comment about the flake to the github object.
func (p *FlakeManager) updateIssue(obj *github.MungeObject, flake cache.Flake) error {
	body := p.flakeID(flake) + "\n" + p.flakeExtraInfo(flake)
	glog.Infof("Updating issue %v with flake %v", *obj.Issue.Number, flake)
	return obj.WriteComment(body)
}

// createIssue makes a new issue for the given flake. If we know about other
// issues for the flake, then they'll be referenced.
func (p *FlakeManager) createIssue(flake cache.Flake) (issueNumber int, err error) {
	body := p.flakeID(flake) + "\n" + p.flakeExtraInfo(flake)
	if previousIssues := p.finder.AllIssuesForKey(string(flake.Test)); len(previousIssues) > 0 {
		s := []string{}
		for _, i := range previousIssues {
			s = append(s, fmt.Sprintf("#%v", i))
		}
		body = body + fmt.Sprintf("\nPrevious issues for this test: %v\n", strings.Join(s, " "))
	}
	obj, err := p.config.NewIssue(
		string(flake.Test), // title
		body,               // body
		[]string{"kind/flake"}, // labels
	)
	if err != nil {
		return 0, err
	}
	glog.Infof("Created issue %v for flake %v", *obj.Issue.Number, flake)
	return *obj.Issue.Number, nil
}
