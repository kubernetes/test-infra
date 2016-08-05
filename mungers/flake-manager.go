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
	"regexp"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/mungegithub/mungers/sync"
	"k8s.io/contrib/mungegithub/mungers/testowner"
	"k8s.io/contrib/test-utils/utils"

	"time"

	"github.com/golang/glog"
	libgithub "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// failedStr is for comment matching during auto prioritization
const failedStr = "Failed: "

var (
	// pullRE is a regexp that will extract the PR# from a path to a flake
	// that happened on a PR.
	pullRE = regexp.MustCompile("pull/([0-9]+)/")
)

// issueFinder finds an issue for a given key.
type issueFinder interface {
	AllIssuesForKey(key string) []int
	Created(key string, number int)
	Synced() bool
}

// FlakeManager files issues for flaky tests.
type FlakeManager struct {
	OwnerPath            string
	finder               issueFinder
	sq                   *SubmitQueue
	config               *github.Config
	googleGCSBucketUtils *utils.Utils
	syncer               *sync.IssueSyncer
	features             *features.Features
}

func init() {
	RegisterMungerOrDie(&FlakeManager{})
}

// Name is the name usable in --pr-mungers
func (p *FlakeManager) Name() string { return "flake-manager" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *FlakeManager) RequiredFeatures() []string { return []string{features.GCSFeature} }

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
	p.googleGCSBucketUtils = utils.NewWithPresubmitDetection(
		features.GCSInfo.BucketName, features.GCSInfo.LogDir,
		features.GCSInfo.PullKey, features.GCSInfo.PullLogDir,
	)

	var owner sync.OwnerMapper
	var err error
	if p.OwnerPath != "" {
		owner, err = testowner.NewReloadingOwnerList(p.OwnerPath)
		if err != nil {
			return err
		}
	}
	p.syncer = sync.NewIssueSyncer(config, p.finder, owner)
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *FlakeManager) EachLoop() error {
	if p.sq.e2e == nil {
		return fmt.Errorf("submit queue not initialized yet")
	}
	if !p.finder.Synced() {
		glog.V(3).Infof("issue-cache is not synced. flake-manager is skipping this run.")
		return nil
	}
	p.sq.e2e.GCSBasedStable()
	for _, f := range p.sq.e2e.Flakes() {
		p.syncFlake(f)
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *FlakeManager) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&p.OwnerPath, "test-owners-csv", "", "file containing a CSV-exported test-owners spreadsheet")
}

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

	if len(f.Result.Flakes) > 0 {
		// If this flake really represents an entire suite failure,
		// this key will be present.
		if _, ok := f.Result.Flakes[cache.RunBrokenTestName]; ok {
			return false
		}
	}

	return true
}

func (p *FlakeManager) listPreviousIssues(title string) []string {
	s := []string{}
	for _, i := range p.finder.AllIssuesForKey(title) {
		s = append(s, fmt.Sprintf("#%v", i))
	}
	return s
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
	id := p.ID()
	extraInfo := fmt.Sprintf(failedStr+"%v\n\n```\n%v\n```\n\n", p.Title(), p.flake.Reason)
	if parts := pullRE.FindStringSubmatch(id); len(parts) > 1 {
		extraInfo += fmt.Sprintf("Happened on a presubmit run in #%v.\n\n", parts[1])
	}
	body := makeGubernatorLink(id) + "\n" + extraInfo

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
	return []string{"kind/flake", sync.PriorityP2.String()}
}

// Priority implements IssueSource
func (p *individualFlakeSource) Priority(obj *github.MungeObject) (sync.Priority, error) {
	comments, err := obj.ListComments()
	if err != nil {
		return sync.PriorityP2, fmt.Errorf("Failed to list comment of issue: %v", err)
	}
	// Different IssueSource's Priority calculation may differ
	return autoPrioritize(comments, obj.Issue.CreatedAt), nil
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
		return fmt.Sprintf(failedStr+"%v\nRun so broken it didn't make JUnit output!", url)
	}
	body := fmt.Sprintf("%v\nMultiple broken tests:\n\n", url)

	sections := []string{}
	for testName, reason := range p.result.Flakes {
		text := fmt.Sprintf(failedStr+"%v\n\n```\n%v\n```\n", testName, reason)
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
	return []string{"kind/flake", "team/test-infra", sync.PriorityP2.String()}
}

// Priority implements IssueSource
func (p *brokenJobSource) Priority(obj *github.MungeObject) (sync.Priority, error) {
	comments, err := obj.ListComments()
	if err != nil {
		return sync.PriorityP2, fmt.Errorf("Failed to list comment of issue: %v", err)
	}
	// Different IssueSource's Priority calculation may differ
	return autoPrioritize(comments, obj.Issue.CreatedAt), nil
}

// autoPrioritize prioritize flake issue based on the number of flakes
func autoPrioritize(comments []*libgithub.IssueComment, issueCreatedAt *time.Time) sync.Priority {
	occurence := []*time.Time{issueCreatedAt}
	lastMonth := time.Now().Add(-1 * 30 * 24 * time.Hour)
	lastWeek := time.Now().Add(-1 * 7 * 24 * time.Hour)
	// number of flakes happened in this month
	monthCount := 0
	// number of flakes happened in this week
	weekCount := 0

	for _, c := range comments {
		// TODO: think of a better way to identify flake comments
		// "Failed: " is a special string contained in flake issue filed by flake-manager
		// Please make sure it matches the body generated by IssueSource.Body()
		if !sync.RobotUser.Has(*c.User.Login) || !strings.Contains(*c.Body, failedStr) {
			continue
		}
		occurence = append(occurence, c.CreatedAt)
	}

	for _, o := range occurence {
		if lastMonth.Before(*o) {
			monthCount += 1
		}
		if lastWeek.Before(*o) {
			weekCount += 1
		}
	}

	// P2: By default
	// P1: Flake happens more than once in last month.
	// P0: Flake happens more than twice in last week.
	p := sync.PriorityP2
	if weekCount >= 3 {
		p = sync.PriorityP0
	} else if monthCount >= 2 {
		p = sync.PriorityP1
	}
	return p
}
