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

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/mungegithub/mungers/sync"
	"k8s.io/contrib/test-utils/utils"

	// "github.com/golang/glog"
	"github.com/spf13/cobra"
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

	syncer *sync.IssueSyncer
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
	p.config = config
	p.googleGCSBucketUtils = utils.NewUtils(utils.KubekinsBucket, utils.LogDir)
	p.syncer = sync.NewIssueSyncer(config, p.finder)
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

func (p *FlakeManager) syncFlake(f cache.Flake) error {
	if p.isIndividualFlake(f) {
		// Just an individual failure.
		return p.syncer.Sync(&individualFlakeSource{f, p})
	}

	return p.syncer.Sync(&brokenJobSource{f.Result, p})
}

func (p *FlakeManager) isIndividualFlake(f cache.Flake) bool {
	// TODO: cache this logic when it gets more complex.
	if f.Result.Status == cache.ResultFailed {
		return false
	}

	// This is the dumbest rule that could possibly be useful.
	// TODO: more robust logic about whether a given flake is a flake or a
	// systemic problem. We should at least account for known flakes before
	// applying this rule.
	if len(f.Result.Flakes) > 3 {
		return false
	}

	return true
}

func (p *FlakeManager) listPreviousIssues(title string) []string {
	if previousIssues := p.finder.AllIssuesForKey(title); len(previousIssues) > 0 {
		s := []string{}
		for _, i := range previousIssues {
			s = append(s, fmt.Sprintf("#%v", i))
		}
		return s
	}
	return nil
}

// makeGubernatorLink returns a URL to view the build results in a GCS path.
//
// gcsPath should be a string like "/kubernetes-jenkins/logs/e2e/1234/",
// pointing at a bucket and path containing test results for a given build.
//
// Gubernator is a simple frontend that reads test result buckets to improve
// test triaging. Its source code is in kubernetes/test-infra/gubernator
func makeGubernatorLink(gcsPath string) string {
	return "https://k8s-gubernator.appspot.com/build" + gcsPath
}

type individualFlakeSource struct {
	flake cache.Flake
	fm    *FlakeManager
}

// Title implements IssueSource
func (p *individualFlakeSource) Title() string {
	// DO NOT CHANGE or it will not recognize previous entries!
	// Note that brokenJobSource.Body() also uses this value to find test
	// flake issues.
	return string(p.flake.Test)
}

// ID implements IssueSource
func (p *individualFlakeSource) ID() string {
	// DO NOT CHANGE or it will not recognize previous entries!
	return p.fm.googleGCSBucketUtils.GetPathToJenkinsGoogleBucket(
		string(p.flake.Job),
		int(p.flake.Number),
	) + "\n"
}

// Body implements IssueSource
func (p *individualFlakeSource) Body(newIssue bool) string {
	extraInfo := fmt.Sprintf("Failed: %v\n\n```\n%v\n```\n\n", p.Title(), p.flake.Reason)
	body := makeGubernatorLink(p.ID()) + "\n" + extraInfo

	if !newIssue {
		return body
	}

	// If we're filing a new issue, reference previous issues if we know of any.
	if s := p.fm.listPreviousIssues(p.Title()); len(s) > 0 {
		body = body + fmt.Sprintf("\nPrevious issues for this test: %v\n", strings.Join(s, " "))
	}
	return body
}

// Labels implements IssueSource
func (p *individualFlakeSource) Labels() []string {
	return []string{"kind/flake"}
}

type brokenJobSource struct {
	result *cache.Result
	fm     *FlakeManager
}

// Title implements IssueSource
func (p *brokenJobSource) Title() string {
	// Keep single issues for test builds and add comments when large
	// batches of failures occur instead of making many issues.
	// DO NOT CHANGE or it will not recognize previous entries!
	return fmt.Sprintf("%v: broken test run", p.result.Job)
}

// ID implements IssueSource
func (p *brokenJobSource) ID() string {
	// DO NOT CHANGE or it will not recognize previous entries!
	return p.fm.googleGCSBucketUtils.GetPathToJenkinsGoogleBucket(
		string(p.result.Job),
		int(p.result.Number),
	) + "\n"
}

// Body implements IssueSource
func (p *brokenJobSource) Body(newIssue bool) string {
	url := makeGubernatorLink(p.ID())
	if p.result.Status == cache.ResultFailed {
		return fmt.Sprintf("%v\nRun so broken it didn't make JUnit output!", url)
	}
	body := fmt.Sprintf("%v\nMultiple broken tests:\n\n", url)

	sections := []string{}
	for testName, reason := range p.result.Flakes {
		text := fmt.Sprintf("Failed: %v\n\n```\n%v\n```\n", testName, reason)
		// Reference previous issues if we know of any.
		// (key must batch individualFlakeSource.Title()!)
		if previousIssues := p.fm.finder.AllIssuesForKey(string(testName)); len(previousIssues) > 0 {
			s := []string{}
			for _, i := range previousIssues {
				s = append(s, fmt.Sprintf("#%v", i))
			}
			text = text + fmt.Sprintf("Issues about this test specifically: %v\n", strings.Join(s, " "))
		}
		sections = append(sections, text)
	}

	body = body + strings.Join(sections, "\n\n")

	if !newIssue {
		return body
	}

	// If we're filing a new issue, reference previous issues if we know of any.
	if s := p.fm.listPreviousIssues(p.Title()); len(s) > 0 {
		body = body + fmt.Sprintf("\nPrevious issues for this suite: %v\n", strings.Join(s, " "))
	}
	return body
}

// Labels implements IssueSource
func (p *brokenJobSource) Labels() []string {
	return []string{"kind/flake", "team/test-infra"}
}
