/*
Copyright 2017 The Kubernetes Authors.

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

package blunderbuss

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pkg/layeredsets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/assign"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "blunderbuss"
)

var (
	match = regexp.MustCompile(`(?mi)^/auto-cc\s*$`)
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequestEvent, helpProvider)
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
}

func configString(reviewCount int) string {
	var pluralSuffix string
	if reviewCount > 1 {
		pluralSuffix = "s"
	}
	return fmt.Sprintf("Blunderbuss is currently configured to request reviews from %d reviewer%s.", reviewCount, pluralSuffix)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	var reviewCount int
	if config.Blunderbuss.ReviewerCount != nil {
		reviewCount = *config.Blunderbuss.ReviewerCount
	}
	two := 2
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Blunderbuss: plugins.Blunderbuss{
			ReviewerCount:         &two,
			MaxReviewerCount:      3,
			ExcludeApprovers:      true,
			UseStatusAvailability: true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The blunderbuss plugin automatically requests reviews from reviewers when a new PR is created. The reviewers are selected based on the reviewers specified in the OWNERS files that apply to the files modified by the PR.",
		Config: map[string]string{
			"": configString(reviewCount),
		},
		Snippet: yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/auto-cc",
		Featured:    false,
		Description: "Manually request reviews from reviewers for a PR. Useful if OWNERS file were updated since the PR was opened.",
		Examples:    []string{"/auto-cc"},
		WhoCanUse:   "Anyone",
	})
	return pluginHelp, nil
}

type reviewersClient interface {
	FindReviewersOwnersForFile(path string) string
	Reviewers(path string) layeredsets.String
	RequiredReviewers(path string) sets.String
	LeafReviewers(path string) sets.String
}

type ownersClient interface {
	reviewersClient
	FindApproverOwnersForFile(path string) string
	Approvers(path string) layeredsets.String
	LeafApprovers(path string) sets.String
}

type fallbackReviewersClient struct {
	ownersClient
}

func (foc fallbackReviewersClient) FindReviewersOwnersForFile(path string) string {
	return foc.ownersClient.FindApproverOwnersForFile(path)
}

func (foc fallbackReviewersClient) Reviewers(path string) layeredsets.String {
	return foc.ownersClient.Approvers(path)
}

func (foc fallbackReviewersClient) LeafReviewers(path string) sets.String {
	return foc.ownersClient.LeafApprovers(path)
}

type githubClient interface {
	RequestReview(org, repo string, number int, logins []string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	Query(context.Context, interface{}, map[string]interface{}) error
}

type repoownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

func handlePullRequestEvent(pc plugins.Agent, pre github.PullRequestEvent) error {
	return handlePullRequest(
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Logger,
		pc.PluginConfig.Blunderbuss,
		pre.Action,
		&pre.PullRequest,
		&pre.Repo,
	)
}

func handlePullRequest(ghc githubClient, roc repoownersClient, log *logrus.Entry, config plugins.Blunderbuss, action github.PullRequestEventAction, pr *github.PullRequest, repo *github.Repo) error {
	if action != github.PullRequestActionOpened || assign.CCRegexp.MatchString(pr.Body) {
		return nil
	}

	return handle(
		ghc,
		roc,
		log,
		config.ReviewerCount,
		config.MaxReviewerCount,
		config.ExcludeApprovers,
		config.UseStatusAvailability,
		repo,
		pr,
	)
}

func handleGenericCommentEvent(pc plugins.Agent, ce github.GenericCommentEvent) error {
	return handleGenericComment(
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Logger,
		pc.PluginConfig.Blunderbuss,
		ce.Action,
		ce.IsPR,
		ce.Number,
		ce.IssueState,
		&ce.Repo,
		ce.Body,
	)
}

func handleGenericComment(ghc githubClient, roc repoownersClient, log *logrus.Entry, config plugins.Blunderbuss, action github.GenericCommentEventAction, isPR bool, prNumber int, issueState string, repo *github.Repo, body string) error {
	if action != github.GenericCommentActionCreated || !isPR || issueState == "closed" {
		return nil
	}

	if !match.MatchString(body) {
		return nil
	}

	pr, err := ghc.GetPullRequest(repo.Owner.Login, repo.Name, prNumber)
	if err != nil {
		return fmt.Errorf("error loading PullRequest: %w", err)
	}

	return handle(
		ghc,
		roc,
		log,
		config.ReviewerCount,
		config.MaxReviewerCount,
		config.ExcludeApprovers,
		config.UseStatusAvailability,
		repo,
		pr,
	)
}

func handle(ghc githubClient, roc repoownersClient, log *logrus.Entry, reviewerCount *int, maxReviewers int, excludeApprovers bool, useStatusAvailability bool, repo *github.Repo, pr *github.PullRequest) error {
	oc, err := roc.LoadRepoOwners(repo.Owner.Login, repo.Name, pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %w", err)
	}

	changes, err := ghc.GetPullRequestChanges(repo.Owner.Login, repo.Name, pr.Number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %w", err)
	}

	var reviewers []string
	var requiredReviewers []string
	if reviewerCount != nil {
		reviewers, requiredReviewers, err = getReviewers(oc, ghc, log, pr.User.Login, changes, *reviewerCount, useStatusAvailability)
		if err != nil {
			return err
		}
		if missing := *reviewerCount - len(reviewers); missing > 0 {
			if !excludeApprovers {
				// Attempt to use approvers as additional reviewers. This must use
				// reviewerCount instead of missing because owners can be both reviewers
				// and approvers and the search might stop too early if it finds
				// duplicates.
				frc := fallbackReviewersClient{ownersClient: oc}
				approvers, _, err := getReviewers(frc, ghc, log, pr.User.Login, changes, *reviewerCount, useStatusAvailability)
				if err != nil {
					return err
				}
				var added int
				combinedReviewers := sets.NewString(reviewers...)
				for _, approver := range approvers {
					if !combinedReviewers.Has(approver) {
						reviewers = append(reviewers, approver)
						combinedReviewers.Insert(approver)
						added++
					}
				}
				log.Infof("Added %d approvers as reviewers. %d/%d reviewers found.", added, combinedReviewers.Len(), *reviewerCount)
			}
		}
		if missing := *reviewerCount - len(reviewers); missing > 0 {
			log.Debugf("Not enough reviewers found in OWNERS files for files touched by this PR. %d/%d reviewers found.", len(reviewers), *reviewerCount)
		}
	}

	if maxReviewers > 0 && len(reviewers) > maxReviewers {
		log.Infof("Limiting request of %d reviewers to %d maxReviewers.", len(reviewers), maxReviewers)
		reviewers = reviewers[:maxReviewers]
	}

	// add required reviewers if any
	reviewers = append(reviewers, requiredReviewers...)

	if len(reviewers) > 0 {
		log.Infof("Requesting reviews from users %s.", reviewers)
		return ghc.RequestReview(repo.Owner.Login, repo.Name, pr.Number, reviewers)
	}
	return nil
}

func getReviewers(rc reviewersClient, ghc githubClient, log *logrus.Entry, author string, files []github.PullRequestChange, minReviewers int, useStatusAvailability bool) ([]string, []string, error) {
	authorSet := sets.NewString(github.NormLogin(author))
	reviewers := layeredsets.NewString()
	requiredReviewers := sets.NewString()
	leafReviewers := layeredsets.NewString()
	fileReviewers := []layeredsets.String{}
	busyReviewers := sets.NewString()
	ownersSeen := sets.NewString()
	if minReviewers == 0 {
		return reviewers.List(), requiredReviewers.List(), nil
	}

	// find statuses of multiple reviewers in one query to respect API rate limits
	if useStatusAvailability {
		reviewersToQuery := prepareLeafReviewersToQuery(rc, ghc, files, authorSet)
		findBusyReviewers(ghc, log, &busyReviewers, reviewersToQuery)
	}

	// build 'reviewers' by taking a unique reviewer from each OWNERS file.
	for _, file := range files {
		ownersFile := rc.FindReviewersOwnersForFile(file.Filename)
		if ownersSeen.Has(ownersFile) {
			continue
		}
		ownersSeen.Insert(ownersFile)

		// record required reviewers if any
		requiredReviewers.Insert(rc.RequiredReviewers(file.Filename).UnsortedList()...)

		fileUnusedLeafs := layeredsets.NewString(rc.LeafReviewers(file.Filename).List()...).Difference(reviewers.Set()).Difference(authorSet)
		if fileUnusedLeafs.Len() == 0 {
			continue
		}
		leafReviewers = leafReviewers.Union(fileUnusedLeafs)
		if r := findReviewer(ghc, useStatusAvailability, busyReviewers, &fileUnusedLeafs); r != "" {
			reviewers.Insert(0, r)
		}
	}

	// ensure that we request review from at least minReviewers reviewers. Favor leaf reviewers.
	unusedLeafs := leafReviewers.Difference(reviewers.Set())
	for reviewers.Len() < minReviewers && unusedLeafs.Len() > 0 {
		if r := findReviewer(ghc, useStatusAvailability, busyReviewers, &unusedLeafs); r != "" {
			reviewers.Insert(1, r)
		}
	}

	for _, file := range files {
		if reviewers.Len() >= minReviewers {
			break
		}
		reviewers := rc.Reviewers(file.Filename).Difference(authorSet)
		fileReviewers = append(fileReviewers, reviewers)
	}

	// find statuses of all remaining reviewers
	if useStatusAvailability {
		reviewersToQuery := layeredsets.NewString()
		for _, list := range fileReviewers {
			reviewersToQuery = reviewersToQuery.Union(list)
		}
		reviewersToQuery = reviewersToQuery.Difference(busyReviewers)
		findBusyReviewers(ghc, log, &busyReviewers, reviewersToQuery.List())
	}

	for _, fromFileReviewers := range fileReviewers {
		for reviewers.Len() < minReviewers && fromFileReviewers.Len() > 0 {
			if r := findReviewer(ghc, useStatusAvailability, busyReviewers, &fromFileReviewers); r != "" {
				reviewers.Insert(2, r)
			}
		}
	}
	return reviewers.List(), requiredReviewers.List(), nil
}

// findReviewer finds a reviewer from a set, potentially using status
// availability.
func findReviewer(ghc githubClient, useStatusAvailability bool, busyReviewers sets.String, targetSet *layeredsets.String) string {
	// if we don't care about status availability, just pop a target from the set
	if !useStatusAvailability {
		return targetSet.PopRandom()
	}

	// if we do care, start looping through the candidates
	for {
		if targetSet.Len() == 0 {
			// if there are no candidates left, then break
			break
		}
		candidate := targetSet.PopRandom()
		if busyReviewers.Has(candidate) {
			// we've already verified this reviewer is busy
			continue
		}
		return candidate
	}
	return ""
}

// prepareLeafReviewersToQuery prepares reviewers list using info from reviewers client crossed with author set
func prepareLeafReviewersToQuery(rc reviewersClient, ghc githubClient, files []github.PullRequestChange, authorSet sets.String) []string {
	ownersSeen := sets.NewString()
	reviewersToQuery := layeredsets.NewString()
	for _, file := range files {
		ownersFile := rc.FindReviewersOwnersForFile(file.Filename)
		if ownersSeen.Has(ownersFile) {
			continue
		}
		ownersSeen.Insert(ownersFile)
		reviewers := layeredsets.NewString(rc.LeafReviewers(file.Filename).List()...).Difference(authorSet)
		reviewersToQuery = reviewersToQuery.Union(reviewers)
	}
	return reviewersToQuery.List()
}

type githubUserAvailability struct {
	Login  string
	Status struct {
		IndicatesLimitedAvailability bool
	}
}

// graphqlTag helps in formating the graphql tag
func graphqlTag(value string) reflect.StructTag {
	return reflect.StructTag(strconv.AppendQuote([]byte("graphql:"), value))
}

// findBusyReviewers finds all the people that are busy and excluded from the review
func findBusyReviewers(ghc githubClient, log *logrus.Entry, busyReviewers *sets.String, reviewers []string) {
	const (
		// will panic if too many fields with too much data in the reflect struct, this setting is very safe
		githubUserAvailabilityStructLimit = 300
	)
	currentIndex := 0
	numberOfBatches := int(math.Ceil(float64(len(reviewers)) / githubUserAvailabilityStructLimit))
	for batch := 0; batch < numberOfBatches; batch++ {
		var fields []reflect.StructField
		structsAdded := 0
		for i := currentIndex; i < len(reviewers) && structsAdded < githubUserAvailabilityStructLimit; i++ {
			fields = append(fields, reflect.StructField{
				Tag:  graphqlTag(fmt.Sprintf("user%d :user(login: %q)", i, reviewers[i])),
				Name: fmt.Sprintf("User%d", i),
				Type: reflect.TypeOf(githubUserAvailability{}),
			})
			structsAdded++
		}
		query := reflect.New(reflect.StructOf(fields)).Elem()
		err := ghc.Query(context.Background(), query.Addr().Interface(), nil)
		if err != nil {
			log.Errorf("error checking users availability: %v", err)
		}
		for i := 0; i < query.NumField(); i++ {
			user := query.Field(i).Interface().(githubUserAvailability)
			if user.Status.IndicatesLimitedAvailability {
				busyReviewers.Insert(user.Login)
			} else if err != nil {
				// when error, count all queried reviewers as busy ones
				busyReviewers.Insert(reviewers[i+currentIndex])
			}
		}
		currentIndex += structsAdded
	}
}
