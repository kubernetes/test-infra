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

package releasenote

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "release-note"

const (
	releaseNoteActionRequired        = "release-note-action-required"
	releaseNoteNone                  = "release-note-none"
	deprecatedReleaseNoteLabelNeeded = "release-note-label-needed"
	releaseNoteLabelNeeded           = "do-not-merge/release-note-label-needed"
	releaseNote                      = "release-note"
)

var (
	allRNLabels = []string{
		releaseNoteNone,
		releaseNoteActionRequired,
		deprecatedReleaseNoteLabelNeeded,
		releaseNoteLabelNeeded,
		releaseNote,
	}

	releaseNoteRe               = regexp.MustCompile(`(?mi)^/release-note\s*$`)
	releaseNoteNoneRe           = regexp.MustCompile(`(?mi)^/release-note-none\s*$`)
	releaseNoteActionRequiredRe = regexp.MustCompile(`(?mi)^/release-note-action-required\s*$`)
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	IsMember(org, user string) (bool, error)
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider PRs and new comments.
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	// Which label does the comment want us to add?
	var nl string
	switch {
	case releaseNoteRe.MatchString(ic.Comment.Body):
		nl = releaseNote
	case releaseNoteNoneRe.MatchString(ic.Comment.Body):
		nl = releaseNoteNone
	case releaseNoteActionRequiredRe.MatchString(ic.Comment.Body):
		nl = releaseNoteActionRequired
	default:
		return nil
	}

	// Only allow authors and org members to add labels.
	isMember, err := gc.IsMember(ic.Repo.Owner.Login, ic.Comment.User.Login)
	if err != nil {
		return err
	}

	isAuthor := ic.Issue.IsAuthor(ic.Comment.User.Login)

	if !isMember && !isAuthor {
		resp := "you can only set release notes if you are the author or an org member."
		log.Infof("Commenting with \"%s\".", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	}

	// Add the requested label if necessary.
	var errs []error
	if !ic.Issue.HasLabel(nl) {
		log.Infof("Adding %s label.", nl)
		if err := gc.AddLabel(org, repo, number, nl); err != nil {
			errs = append(errs, err)
		}
	}
	// Remove all other release-note-* labels if necessary.
	for _, l := range allRNLabels {
		if l != nl && ic.Issue.HasLabel(l) {
			log.Infof("Removing %s label.", l)
			if err := gc.RemoveLabel(org, repo, number, l); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors setting labels: %v", len(errs), errs)
	}
	return nil
}
