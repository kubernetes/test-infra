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
	"time"

	"github.com/golang/lint"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/genfiles"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/golint/suggestion"
)

const (
	pluginName  = "golint"
	commentTag  = "<!-- golint -->"
	maxComments = 20
)

var lintRe = regexp.MustCompile(`(?mi)^/lint\s*$`)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The golint plugin runs golint on changes made to *.go files in a PR. It then creates a new review on the pull request and leaves golint warnings at the appropriate lines of code.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/lint",
		Featured:    false,
		Description: "Runs golint on changes made to *.go files in a PR",
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/lint"},
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

const defaultConfidence = 0.8

func minConfidence(g *plugins.Golint) float64 {
	if g == nil || g.MinimumConfidence == nil {
		return defaultConfidence
	}
	return *g.MinimumConfidence
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(minConfidence(pc.PluginConfig.Golint), pc.GitHubClient, pc.GitClient, pc.Logger, &e)
}

// modifiedGoFiles returns a map from filename to patch string for all go files
// that are modified in the PR excluding vendor/ and generated files.
func modifiedGoFiles(ghc githubClient, org, repo string, number int, sha string) (map[string]string, error) {
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
		case strings.HasPrefix(change.Filename, "vendor/"):
			continue
		case filepath.Ext(change.Filename) != ".go":
			continue
		case gfg.Match(change.Filename):
			continue
		case change.Status == github.PullRequestFileRemoved || change.Status == github.PullRequestFileRenamed:
			continue
		}
		modifiedFiles[change.Filename] = change.Patch
	}
	return modifiedFiles, nil
}

// newProblems compares the list of problems with the list of past comments on
// the PR to decide which are new.
func newProblems(cs []github.ReviewComment, ps map[string]map[int]lint.Problem) map[string]map[int]lint.Problem {
	// Make a copy, then remove the old elements.
	res := make(map[string]map[int]lint.Problem)
	for f, ls := range ps {
		res[f] = make(map[int]lint.Problem)
		for l, p := range ls {
			res[f][l] = p
		}
	}
	for _, c := range cs {
		if c.Position == nil {
			continue
		}
		if !strings.Contains(c.Body, commentTag) {
			continue
		}
		delete(res[c.Path], *c.Position)
	}
	return res
}

// problemsInFiles runs golint on the files. It returns a map from the file to
// a map from the line in the patch to the problem.
func problemsInFiles(r *git.Repo, files map[string]string) (map[string]map[int]lint.Problem, error) {
	problems := make(map[string]map[int]lint.Problem)
	l := new(lint.Linter)
	for f, patch := range files {
		problems[f] = make(map[int]lint.Problem)
		src, err := ioutil.ReadFile(filepath.Join(r.Dir, f))
		if err != nil {
			return nil, err
		}
		ps, err := l.Lint(f, src)
		if err != nil {
			return nil, fmt.Errorf("linting %s: %v", f, err)
		}
		al, err := AddedLines(patch)
		if err != nil {
			return nil, fmt.Errorf("computing added lines in %s: %v", f, err)
		}
		for _, p := range ps {
			if pl, ok := al[p.Position.Line]; ok {
				problems[f][pl] = p
			}
		}
	}
	return problems, nil
}

func handle(minimumConfidence float64, ghc githubClient, gc *git.Client, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only handle open PRs and new requests.
	if e.IssueState != "open" || !e.IsPR || e.Action != github.GenericCommentActionCreated {
		return nil
	}
	if !lintRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name

	pr, err := ghc.GetPullRequest(org, repo, e.Number)
	if err != nil {
		return err
	}

	// List modified files.
	modifiedFiles, err := modifiedGoFiles(ghc, org, repo, pr.Number, pr.Head.SHA)
	if err != nil {
		return err
	}
	if len(modifiedFiles) == 0 {
		return nil
	}
	log.Infof("Will lint %d modified go files.", len(modifiedFiles))

	// Clone the repo, checkout the PR.
	startClone := time.Now()
	r, err := gc.Clone(e.Repo.FullName)
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

	// Compute lint errors.
	problems, err := problemsInFiles(r, modifiedFiles)
	if err != nil {
		return err
	}
	// Filter out problems that are below our threshold
	for file := range problems {
		for line, problem := range problems[file] {
			if problem.Confidence < minimumConfidence {
				delete(problems[file], line)
			}
		}
	}
	log.WithField("duration", time.Since(finishClone)).Info("Linted.")

	oldComments, err := ghc.ListPullRequestComments(org, repo, e.Number)
	if err != nil {
		return err
	}
	nps := newProblems(oldComments, problems)

	// Make the list of comments.
	var comments []github.DraftReviewComment
	for f, ls := range nps {
		for l, p := range ls {
			var suggestion = suggestion.SuggestCodeChange(p)
			var body string
			var link string
			if p.Link != "" {
				link = fmt.Sprintf("[More info](%s). ", p.Link)
			}
			body = fmt.Sprintf("%sGolint %s: %s. %s%s", suggestion, p.Category, p.Text, link, commentTag)
			comments = append(comments, github.DraftReviewComment{
				Path:     f,
				Position: l,
				Body:     body,
			})
		}
	}

	// Trim down the number of comments if necessary.
	totalProblems := numProblems(problems)
	newProblems := numProblems(nps)
	oldProblems := totalProblems - newProblems

	allowedComments := maxComments - oldProblems
	if allowedComments < 0 {
		allowedComments = 0
	}
	if len(comments) > allowedComments {
		comments = comments[:allowedComments]
	}

	// Make the review body.
	s := "s"
	if totalProblems == 1 {
		s = ""
	}

	response := fmt.Sprintf("%d warning%s.", totalProblems, s)

	if oldProblems != 0 {
		response = fmt.Sprintf("%d unresolved warning%s and %d new warning%s.", oldProblems, s, newProblems, s)
	}

	return ghc.CreateReview(org, repo, e.Number, github.DraftReview{
		Body:     plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, response),
		Action:   github.Comment,
		Comments: comments,
	})
}

func numProblems(ps map[string]map[int]lint.Problem) int {
	var num int
	for _, m := range ps {
		num += len(m)
	}
	return num
}

// AddedLines returns line numbers that were added in the patch, along with
// their line in the patch itself as a map from line to patch line.
// https://www.gnu.org/software/diffutils/manual/diffutils.html#Detailed-Unified
// GitHub omits the ---/+++ lines since that information is in the
// PullRequestChange object.
func AddedLines(patch string) (map[int]int, error) {
	result := make(map[int]int)
	if patch == "" {
		return result, nil
	}
	lines := strings.Split(patch, "\n")
	for i := 0; i < len(lines); i++ {
		// dodge the "\ No newline at end of file" line
		if lines[i] == "\\ No newline at end of file" {
			continue
		}
		_, oldLen, newLine, newLen, err := parseHunkLine(lines[i])
		if err != nil {
			return nil, fmt.Errorf("couldn't parse hunk on line %d in patch %s: %v", i, patch, err)
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
				result[newLine+newAdd] = i
				newAdd++
			default:
				return nil, fmt.Errorf("bad prefix on line %d in patch %s", i, patch)
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
