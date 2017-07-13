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

package golint

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/golang/lint"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "golint"

var lintRe = regexp.MustCompile(`(?mi)^/lint\s*$`)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIC)
}

type githubClient interface {
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	CreateComment(org, repo string, number int, body string) error
}

func handleIC(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.GitClient, pc.Logger, ic)
}

// modifiedGoFiles returns a map from filename to patch string for all go files
// that are modified in the PR.
func modifiedGoFiles(ghc githubClient, org, repo string, number int) (map[string]string, error) {
	changes, err := ghc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		return nil, err
	}

	modifiedFiles := make(map[string]string)
	for _, change := range changes {
		if filepath.Ext(change.Filename) == ".go" {
			modifiedFiles[change.Filename] = change.Patch
		}
	}
	return modifiedFiles, nil
}

// problemsInPatches runs golint on the files. It returns all lint problems
// that are in added lines in the files' patches.
func problemsInFiles(r *git.Repo, files map[string]string) ([]lint.Problem, error) {
	var problems []lint.Problem
	l := new(lint.Linter)
	for f, patch := range files {
		src, err := ioutil.ReadFile(filepath.Join(r.Dir, f))
		if err != nil {
			return nil, err
		}
		ps, err := l.Lint(f, src)
		if err != nil {
			return nil, err
		}
		al, err := addedLines(patch)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if al[p.Position.Line] {
				problems = append(problems, p)
			}
		}
	}
	return problems, nil
}

func handle(ghc githubClient, gc *git.Client, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only handle open PRs and new requests.
	if ic.Issue.State != "open" || !ic.Issue.IsPullRequest() || ic.Action != "created" {
		return nil
	}
	if !lintRe.MatchString(ic.Comment.Body) {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name

	// List modified files.
	modifiedFiles, err := modifiedGoFiles(ghc, org, repo, ic.Issue.Number)
	if err != nil {
		return err
	}
	if len(modifiedFiles) == 0 {
		return nil
	}

	// Clone the repo, checkout the PR.
	r, err := gc.Clone(ic.Repo.FullName)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.CheckoutPullRequest(ic.Issue.Number); err != nil {
		return err
	}

	// Compute lint errors.
	problems, err := problemsInFiles(r, modifiedFiles)
	if err != nil {
		return err
	}

	// Respond.
	var response string
	if len(problems) == 0 {
		response = "no lint warnings"
	} else {
		response = fmt.Sprintf("%d warning(s):\n\n", len(problems))
		var warnings []string
		for _, p := range problems {
			warnings = append(warnings, fmt.Sprintf("`%s:%d`: %s", p.Position.Filename, p.Position.Line, p.Text))
		}
		response += strings.Join(warnings, "\n")
	}

	return ghc.CreateComment(org, repo, ic.Issue.Number, plugins.FormatICResponse(ic.Comment, response))
}

// addedLines returns a set of line numbers that were added in the patch.
// https://www.gnu.org/software/diffutils/manual/diffutils.html#Detailed-Unified
// GitHub omits the ---/+++ lines since that information is in the
// PullRequestChange object.
func addedLines(patch string) (map[int]bool, error) {
	result := make(map[int]bool)
	lines := strings.Split(patch, "\n")
	for i := 0; i < len(lines); i++ {
		_, oldLen, newLine, newLen, err := parseHunkLine(lines[i])
		if err != nil {
			return nil, err
		}
		oldAdd := 0
		newAdd := 0
		for oldAdd < oldLen || newAdd < newLen {
			i++
			if i >= len(lines) {
				return nil, fmt.Errorf("invalid patch: %s", patch)
			}
			switch lines[i][0] {
			case ' ':
				oldAdd++
				newAdd++
			case '-':
				oldAdd++
			case '+':
				result[newLine+newAdd] = true
				newAdd++
			default:
				return nil, fmt.Errorf("bad line in patch: %s", lines[i])
			}
		}
	}
	return result, nil
}

// Matches the hunk line in unified diffs. These are of the form:
// @@ -l,s +l,s @@ section head
// We need to extract the four numbers, but the command and s is optional.
// See https://en.wikipedia.org/wiki/Diff_utility#Unified_format
var hunkRe = regexp.MustCompile(`^@@ -(\d+),?(\d+)? \+(\d+),?(\d+)? @@.*`)

func parseHunkLine(hunk string) (oldLine, oldLength, newLine, newLength int, err error) {
	if !hunkRe.MatchString(hunk) {
		err = fmt.Errorf("invalid hunk line: %s", hunk)
		return
	}
	matches := hunkRe.FindStringSubmatch(hunk)
	oldLine, err = strconv.Atoi(matches[1])
	if err != nil {
		return
	}
	if matches[2] != "" {
		oldLength, err = strconv.Atoi(matches[2])
		if err != nil {
			return
		}
	} else {
		oldLength = 1
	}
	newLine, err = strconv.Atoi(matches[3])
	if err != nil {
		return
	}
	if matches[4] != "" {
		newLength, err = strconv.Atoi(matches[4])
		if err != nil {
			return
		}
	} else {
		newLength = 1
	}
	return
}
