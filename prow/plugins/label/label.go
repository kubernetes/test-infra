/*
Copyright 2016 The Kubernetes Authors.

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

package label

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "label"

type assignEvent struct {
	body    string
	login   string
	org     string
	repo    string
	url     string
	number  int
	issue   github.Issue
	comment github.IssueComment
}

var (
	labelRegex              = regexp.MustCompile(`(?m)^/(area|priority|kind|sig)\s*(.*)$`)
	removeLabelRegex        = regexp.MustCompile(`(?m)^/remove-(area|priority|kind|sig)\s*(.*)$`)
	statusRegex             = regexp.MustCompile(`(?m)^/status\s+(.+)$`)
	sigMatcher              = regexp.MustCompile(`(?m)@kubernetes/sig-([\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)`)
	chatBack                = "Reiterating the mentions to trigger a notification: \n%v"
	nonExistentLabelOnIssue = "Those labels are not set on the issue: `%v`"
	mustBeSigLead           = "You must be a member of the @kubernetes/kubernetes-milestone-maintainers github team to add status labels"
	statusMap               = map[string]string{
		"approved-for-milestone": "status/approved-for-milestone",
		"in-progress":            "status/in-progress",
		"in-review":              "status/in-review",
	}
	kindMap = map[string]string{
		"bugs":             "kind/bug",
		"feature-requests": "kind/feature",
		"api-reviews":      "kind/api-change",
		"proposals":        "kind/design",
	}
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterIssueHandler(pluginName, handleIssue)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetRepoLabels(owner, repo string) ([]github.Label, error)
	BotName() (string, error)
	ListTeamMembers(id int) ([]github.TeamMember, error)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	if ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	ae := assignEvent{
		body:    ic.Comment.Body,
		login:   ic.Comment.User.Login,
		org:     ic.Repo.Owner.Login,
		repo:    ic.Repo.Name,
		url:     ic.Comment.HTMLURL,
		number:  ic.Issue.Number,
		issue:   ic.Issue,
		comment: ic.Comment,
	}
	return handle(pc.GitHubClient, pc.Logger, ae, pc.PluginConfig.Label.MilestoneMaintainersID)
}

func handleIssue(pc plugins.PluginClient, i github.IssueEvent) error {
	if i.Action != github.IssueActionOpened {
		return nil
	}

	ae := assignEvent{
		body:   i.Issue.Body,
		login:  i.Issue.User.Login,
		org:    i.Repo.Owner.Login,
		repo:   i.Repo.Name,
		url:    i.Issue.HTMLURL,
		number: i.Issue.Number,
		issue:  i.Issue,
	}
	return handle(pc.GitHubClient, pc.Logger, ae, pc.PluginConfig.Label.MilestoneMaintainersID)
}

func handlePullRequest(pc plugins.PluginClient, pr github.PullRequestEvent) error {
	if pr.Action != github.PullRequestActionOpened {
		return nil
	}

	ae := assignEvent{
		body:   pr.PullRequest.Body,
		login:  pr.PullRequest.User.Login,
		org:    pr.PullRequest.Base.Repo.Owner.Login,
		repo:   pr.PullRequest.Base.Repo.Name,
		url:    pr.PullRequest.HTMLURL,
		number: pr.Number,
	}
	return handle(pc.GitHubClient, pc.Logger, ae, pc.PluginConfig.Label.MilestoneMaintainersID)
}

// Get Labels from Regexp matches
func getLabelsFromREMatches(matches [][]string) (labels []string) {
	for _, match := range matches {
		for _, label := range strings.Split(match[0], " ")[1:] {
			label = strings.ToLower(match[1] + "/" + strings.TrimSpace(label))
			labels = append(labels, label)
		}
	}
	return
}

func (ae assignEvent) getRepeats(sigMatches [][]string, existingLabels map[string]string) (toRepeat []string) {
	toRepeat = []string{}
	for _, sigMatch := range sigMatches {
		sigLabel := strings.ToLower("sig" + "/" + strings.TrimSpace(sigMatch[1]))

		if _, ok := existingLabels[sigLabel]; ok {
			toRepeat = append(toRepeat, sigMatch[0])
		}
	}
	return
}

// TODO: refactor this function.  It's grown too complex
func handle(gc githubClient, log *logrus.Entry, ae assignEvent, maintainersID int) error {
	// only parse newly created comments/issues/PRs and if non bot author
	botName, err := gc.BotName()
	if err != nil {
		return err
	}
	if ae.login == botName {
		return nil
	}

	labelMatches := labelRegex.FindAllStringSubmatch(ae.body, -1)
	removeLabelMatches := removeLabelRegex.FindAllStringSubmatch(ae.body, -1)
	sigMatches := sigMatcher.FindAllStringSubmatch(ae.body, -1)
	statusMatches := statusRegex.FindAllStringSubmatch(ae.body, -1)
	if len(labelMatches) == 0 && len(sigMatches) == 0 && len(removeLabelMatches) == 0 && len(statusMatches) == 0 {
		return nil
	}

	labels, err := gc.GetRepoLabels(ae.org, ae.repo)
	if err != nil {
		return err
	}

	existingLabels := map[string]string{}
	for _, l := range labels {
		existingLabels[strings.ToLower(l.Name)] = l.Name
	}
	var (
		nonexistent         []string
		noSuchLabelsOnIssue []string
		labelsToAdd         []string
		labelsToRemove      []string
	)

	// Get labels to add and labels to remove from regexp matches
	labelsToAdd = getLabelsFromREMatches(labelMatches)
	labelsToRemove = getLabelsFromREMatches(removeLabelMatches)

	// Add labels
	for _, labelToAdd := range labelsToAdd {
		if ae.issue.HasLabel(labelToAdd) {
			continue
		}

		if _, ok := existingLabels[labelToAdd]; !ok {
			nonexistent = append(nonexistent, labelToAdd)
			continue
		}

		if err := gc.AddLabel(ae.org, ae.repo, ae.number, existingLabels[labelToAdd]); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", labelToAdd)
		}
	}

	// Remove labels
	for _, labelToRemove := range labelsToRemove {
		if !ae.issue.HasLabel(labelToRemove) {
			noSuchLabelsOnIssue = append(noSuchLabelsOnIssue, labelToRemove)
			continue
		}

		if _, ok := existingLabels[labelToRemove]; !ok {
			nonexistent = append(nonexistent, labelToRemove)
			continue
		}

		if err := gc.RemoveLabel(ae.org, ae.repo, ae.number, labelToRemove); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", labelToRemove)
		}
	}

	maintainersMap := map[string]bool{}
	milestoneMaintainers, err := gc.ListTeamMembers(maintainersID)
	if err != nil {
		log.WithError(err).Errorf("Failed to list the teammembers for the milestone maintainers team")
	} else {
		for _, person := range milestoneMaintainers {
			maintainersMap[person.Login] = true
		}
	}

	for _, statusMatch := range statusMatches {
		status := strings.TrimSpace(statusMatch[1])
		sLabel, validStatus := statusMap[status]
		if validStatus {
			_, ok := maintainersMap[ae.login]
			if ok {
				if err := gc.AddLabel(ae.org, ae.repo, ae.number, sLabel); err != nil {
					log.WithError(err).Errorf("Github failed to add the following label: %s", sLabel)
				}
			} else {
				// not in the milestone maintainers team
				if err := gc.CreateComment(ae.org, ae.repo, ae.number, mustBeSigLead); err != nil {
					log.WithError(err).Errorf("Could not create comment \"%s\".", mustBeSigLead)
				}
			}

		}
	}

	for _, sigMatch := range sigMatches {
		sigLabel := strings.ToLower("sig" + "/" + strings.TrimSpace(sigMatch[1]))
		kind := sigMatch[2]
		if ae.issue.HasLabel(sigLabel) {
			continue
		}
		if _, ok := existingLabels[sigLabel]; !ok {
			nonexistent = append(nonexistent, sigLabel)
			continue
		}
		if err := gc.AddLabel(ae.org, ae.repo, ae.number, sigLabel); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", sigLabel)
		}

		if kindLabel, ok := kindMap[kind]; ok {
			if err := gc.AddLabel(ae.org, ae.repo, ae.number, kindLabel); err != nil {
				log.WithError(err).Errorf("Github failed to add the following label: %s", kindLabel)
			}
		}
	}

	toRepeat := []string{}
	isMember := false
	if len(sigMatches) > 0 {
		isMember, err = gc.IsMember(ae.org, ae.login)
		if err != nil {
			log.WithError(err).Errorf("Github error occurred when checking if the user: %s is a member of org: %s.", ae.login, ae.org)
		}
		toRepeat = ae.getRepeats(sigMatches, existingLabels)
	}

	if len(toRepeat) > 0 {
		if !isMember {
			msg := fmt.Sprintf(chatBack, strings.Join(toRepeat, ", "))
			if err := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); err != nil {
				log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
			}
		}
	}

	//TODO(grodrigues3): Once labels are standardized, make this reply with a comment.
	if len(nonexistent) > 0 {
		log.Infof("Nonexistent labels: %v", nonexistent)
	}

	// Tried to remove Labels that were not present on the Issue
	if len(noSuchLabelsOnIssue) > 0 {
		msg := fmt.Sprintf(nonExistentLabelOnIssue, strings.Join(noSuchLabelsOnIssue, ", "))
		if err := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		}
	}

	return nil
}
