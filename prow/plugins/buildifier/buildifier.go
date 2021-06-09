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

// Package buildifier defines a Prow plugin that runs buildifier over modified
// BUILD, WORKSPACE, and skylark (.bzl) files in pull requests.
package buildifier

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bazelbuild/buildtools/build"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/genfiles"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "buildifier"
)

var buildifyRe = regexp.MustCompile(`(?mi)^/buildif(y|ier)\s*$`)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The buildifier plugin runs buildifier on changes made to Bazel files in a PR. It then creates a new review on the pull request and leaves warnings at the appropriate lines of code.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/buildif(y|ier)",
		Featured:    false,
		Description: "Runs buildifier on changes made to Bazel files in a PR",
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/buildify", "/buildifier"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	CreateReview(org, repo string, number int, r github.DraftReview) error
	ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error)
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.GitClient, pc.Logger, &e)
}

// modifiedBazelFiles returns a map from filename to patch string for all Bazel files
// that are modified in the PR.
func modifiedBazelFiles(ghc githubClient, org, repo string, number int, sha string) (map[string]string, error) {
	changes, err := ghc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		return nil, err
	}

	gfg, err := genfiles.NewGroup(ghc, org, repo, sha)
	if err != nil {
		return nil, err
	}

	modifiedFiles := make(map[string]string)
	for _, change := range changes {
		switch {
		case gfg.Match(change.Filename):
			continue
		case change.Status == github.PullRequestFileRemoved || change.Status == github.PullRequestFileRenamed:
			continue
		// This also happens to match BUILD.bazel.
		case strings.Contains(change.Filename, "BUILD"):
			break
		case strings.Contains(change.Filename, "WORKSPACE"):
			break
		case filepath.Ext(change.Filename) != ".bzl":
			continue
		}
		modifiedFiles[change.Filename] = change.Patch
	}
	return modifiedFiles, nil
}

// problemsInFiles runs buildifier on the files. It returns a map from the file to
// a list of problems with that file.
func problemsInFiles(r git.RepoClient, files map[string]string) (map[string]bool, error) {
	problems := map[string]bool{}
	for f := range files {
		src, err := ioutil.ReadFile(filepath.Join(r.Directory(), f))
		if err != nil {
			return nil, err
		}
		// This is modeled after the logic from buildifier:
		// https://github.com/bazelbuild/buildtools/blob/8818289/buildifier/buildifier.go#L261
		content, err := build.Parse(f, src)
		if err != nil {
			return nil, fmt.Errorf("parsing as Bazel file %v", err)
		}
		beforeRewrite := build.FormatWithoutRewriting(content)
		ndata := build.Format(content)
		if !bytes.Equal(src, ndata) && !bytes.Equal(src, beforeRewrite) {
			problems[f] = true
		}
	}
	return problems, nil
}

func handle(ghc githubClient, gc git.ClientFactory, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only handle open PRs and new requests.
	if e.IssueState != "open" || !e.IsPR || e.Action != github.GenericCommentActionCreated {
		return nil
	}
	if !buildifyRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name

	pr, err := ghc.GetPullRequest(org, repo, e.Number)
	if err != nil {
		return err
	}

	// List modified files.
	modifiedFiles, err := modifiedBazelFiles(ghc, org, repo, pr.Number, pr.Head.SHA)
	if err != nil {
		return err
	}
	if len(modifiedFiles) == 0 {
		return nil
	}
	log.Infof("Will buildify %d modified Bazel files.", len(modifiedFiles))

	// Clone the repo, checkout the PR.
	startClone := time.Now()
	r, err := gc.ClientFor(org, repo)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.CheckoutPullRequest(e.Number); err != nil {
		return err
	}
	finishClone := time.Now()
	log.WithField("duration", time.Since(startClone)).Info("Cloned and checked out PR.")

	// Compute buildifier errors.
	problems, err := problemsInFiles(r, modifiedFiles)
	if err != nil {
		return err
	}
	log.WithField("duration", time.Since(finishClone)).Info("Buildified.")

	// Make the list of comments.
	var comments []github.DraftReviewComment
	for f := range problems {
		comments = append(comments, github.DraftReviewComment{
			Path: f,
			// TODO(mattmoor): Include the messages if they are ever non-empty.
			Body: strings.Join([]string{
				"This Bazel file needs formatting, run:",
				"```shell",
				fmt.Sprintf("buildifier -mode=fix %q", f),
				"```"}, "\n"),
			Position: 1,
		})
	}

	// Trim down the number of comments if necessary.
	totalProblems := len(problems)

	// Make the review body.
	s := "s"
	if totalProblems == 1 {
		s = ""
	}
	response := fmt.Sprintf("%d warning%s.", totalProblems, s)

	return ghc.CreateReview(org, repo, e.Number, github.DraftReview{
		Body:     plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, response),
		Action:   github.Comment,
		Comments: comments,
	})
}
