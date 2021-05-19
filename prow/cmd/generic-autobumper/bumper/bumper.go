/*
Copyright 2019 The Kubernetes Authors.

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

package bumper

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const (
	forkRemoteName = "bumper-fork-remote"

	defaultHeadBranchName = "autobump"
	defaultOncallGroup    = "testinfra"

	errOncallMsgTempl = "An error occurred while finding an assignee: `%s`.\nFalling back to Blunderbuss."
	noOncallMsg       = "Nobody is currently oncall, so falling back to Blunderbuss."

	gitCmd = "git"
)

type fileArrayFlag []string

func (af *fileArrayFlag) String() string {
	return fmt.Sprint(*af)
}

func (af *fileArrayFlag) Set(value string) error {
	for _, e := range strings.Split(value, ",") {
		fn := strings.TrimSpace(e)
		info, err := os.Stat(fn)
		if err != nil {
			return fmt.Errorf("getting file info for %q", fn)
		}
		if info.IsDir() && !strings.HasSuffix(fn, string(os.PathSeparator)) {
			fn = fn + string(os.PathSeparator)
		}
		*af = append(*af, fn)
	}
	return nil
}

// Options is the options for autobumper operations.
type Options struct {
	// The target GitHub org name where the autobump PR will be created. Only required when SkipPullRequest is false.
	GitHubOrg string `json:"gitHubOrg"`
	// The target GitHub repo name where the autobump PR will be created. Only required when SkipPullRequest is false.
	GitHubRepo string `json:"gitHubRepo"`
	// The name of the branch in the target GitHub repo on which the autobump PR will be based.  If not specified, will be autodetected via GitHub API.
	GitHubBaseBranch string `json:"gitHubBaseBranch"`
	// The GitHub username to use. If not specified, uses values from the user associated with the access token.
	GitHubLogin string `json:"gitHubLogin"`
	// The path to the GitHub token file. Only required when SkipPullRequest is false.
	GitHubToken string `json:"gitHubToken"`
	// The name to use on the git commit. Only required when GitEmail is specified and SkipPullRequest is false. If not specified, uses values from the user associated with the access token
	GitName string `json:"gitName"`
	// The email to use on the git commit. Only required when GitName is specified and SkipPullRequest is false. If not specified, uses values from the user associated with the access token.
	GitEmail string `json:"gitEmail"`
	// AssignTo specifies who to assign the created PR to. Takes precedence over onCallAddress and onCallGroup if set.
	AssignTo string `json:"assign_to"`
	// The oncall address where we can get the JSON file that stores the current oncall information.
	OncallAddress string `json:"onCallAddress"`
	// The oncall group that is responsible for reviewing the change, i.e. "test-infra".
	OncallGroup string `json:"onCallGroup"`
	// Whether to skip creating the pull request for this bump.
	SkipPullRequest bool `json:"skipPullRequest"`
	// Information needed to do a gerrit bump. Do not include if doing github bump
	Gerrit *Gerrit `json:"gerrit"`
	// The name used in the address when creating remote. This should be the same name as the fork. If fork does not exist this will be the name of the fork that is created.
	// If it is not the same as the fork, the robot will change the name of the fork to this. Format will be git@github.com:{GitLogin}/{RemoteName}.git
	RemoteName string `json:"remoteName"`
	// The name of the branch that will be used when creating the pull request. If unset, defaults to "autobump".
	HeadBranchName string `json:"headBranchName"`
	// Optional list of labels to add to the bump PR
	Labels []string `json:"labels"`
}

// Information needed for gerrit bump
type Gerrit struct {
	// Unique tag in commit messages to identify a Gerrit bump CR. Required if using gerrit
	AutobumpPRIdentifier string `json:"autobumpPRIdentifier"`
	// Gerrit CR Author. Only Required if using gerrit
	Author string `json:"author"`
	// Email account associated with gerrit author. Only required if using gerrit.
	Email string `json:"email"`
	// The path to the Gerrit httpcookie file. Only Required if using gerrit
	CookieFile string `json:"cookieFile"`
	// The path to the hosted Gerrit repo
	HostRepo string `json:"hostRepo"`
}

// PRHandler is the interface implemented by consumer of prcreator, for
// manipulating the repo, and provides commit messages, PR title and body.
type PRHandler interface {
	// Changes returns a slice of functions, each one does some stuff, and
	// returns commit message for the changes
	Changes() []func() (string, error)
	// PRTitleBody returns the body of the PR, this function runs after all
	// changes have been executed
	PRTitleBody() (string, string, error)
}

// GitAuthorOptions is specifically to read the author info for a commit
type GitAuthorOptions struct {
	GitName  string
	GitEmail string
}

// AddFlags will read the author info from the command line parameters
func (o *GitAuthorOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.GitName, "git-name", "", "The name to use on the git commit.")
	fs.StringVar(&o.GitEmail, "git-email", "", "The email to use on the git commit.")
}

// Validate will validate the input GitAuthorOptions
func (o *GitAuthorOptions) Validate() error {
	if (o.GitEmail == "") != (o.GitName == "") {
		return fmt.Errorf("--git-name and --git-email must be specified together")
	}
	return nil
}

// GitCommand is used to pass the various components of the git command which needs to be executed
type GitCommand struct {
	baseCommand string
	args        []string
	workingDir  string
}

// Call will execute the Git command and switch the working directory if specified
func (gc GitCommand) Call(stdout, stderr io.Writer) error {
	return Call(stdout, stderr, gc.baseCommand, gc.buildCommand()...)
}

func (gc GitCommand) buildCommand() []string {
	args := []string{}
	if gc.workingDir != "" {
		args = append(args, "-C", gc.workingDir)
	}
	args = append(args, gc.args...)
	return args
}

func (gc GitCommand) getCommand() string {
	return fmt.Sprintf("%s %s", gc.baseCommand, strings.Join(gc.buildCommand(), " "))
}

func validateOptions(o *Options) error {
	if !o.SkipPullRequest && o.Gerrit == nil {
		if o.GitHubToken == "" {
			return fmt.Errorf("gitHubToken is mandatory when skipPullRequest is false or unspecified")
		}
		if (o.GitEmail == "") != (o.GitName == "") {
			return fmt.Errorf("gitName and gitEmail must be specified together")
		}
		if o.GitHubOrg == "" || o.GitHubRepo == "" {
			return fmt.Errorf("gitHubOrg and gitHubRepo are mandatory when skipPullRequest is false or unspecified")
		}
		if o.RemoteName == "" {
			return fmt.Errorf("remoteName is mandatory when skipPullRequest is false or unspecified")
		}
	}
	if !o.SkipPullRequest && o.Gerrit != nil {
		if o.Gerrit.Author == "" {
			return fmt.Errorf("GerritAuthor is required when skipPullRequest is false and Gerrit is true")
		}
		if o.Gerrit.AutobumpPRIdentifier == "" {
			return fmt.Errorf("GerritCommitId is required when skipPullRequest is false and Gerrit is true")
		}
		if o.Gerrit.HostRepo == "" {
			return fmt.Errorf("GerritHostRepo is required when skipPullRequest is false and Gerrit is true")
		}
		if o.Gerrit.CookieFile == "" {
			return fmt.Errorf("GerritCookieFile is required when skipPullRequest is false and Gerrit is true")
		}
	}
	if !o.SkipPullRequest {
		if o.HeadBranchName == "" {
			o.HeadBranchName = defaultHeadBranchName
		}
	}
	if o.OncallGroup == "" {
		o.OncallGroup = defaultOncallGroup
	}

	return nil
}

// Run is the entrypoint which will update Prow config files based on the
// provided options.
//
// updateFunc: a function that returns commit message and error
func Run(o *Options, prh PRHandler) error {
	if err := validateOptions(o); err != nil {
		return fmt.Errorf("validating options: %w", err)
	}

	if o.SkipPullRequest {
		logrus.Debugf("--skip-pull-request is set to true, won't create a pull request.")
	}
	if o.Gerrit == nil {
		return processGitHub(o, prh)
	}
	return processGerrit(o, prh)
}

func processGitHub(o *Options, prh PRHandler) error {
	var sa secret.Agent

	stdout := HideSecretsWriter{Delegate: os.Stdout, Censor: &sa}
	stderr := HideSecretsWriter{Delegate: os.Stderr, Censor: &sa}
	if err := sa.Start([]string{o.GitHubToken}); err != nil {
		return fmt.Errorf("start secrets agent: %w", err)
	}

	gc := github.NewClient(sa.GetTokenGenerator(o.GitHubToken), sa.Censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

	if o.GitHubLogin == "" || o.GitName == "" || o.GitEmail == "" {
		user, err := gc.BotUser()
		if err != nil {
			return fmt.Errorf("get the user data for the provided GH token: %w", err)
		}
		if o.GitHubLogin == "" {
			o.GitHubLogin = user.Login
		}
		if o.GitName == "" {
			o.GitName = user.Name
		}
		if o.GitEmail == "" {
			o.GitEmail = user.Email
		}
	}

	// Make change, commit and push
	var anyChange bool
	for i, changeFunc := range prh.Changes() {
		msg, err := changeFunc()
		if err != nil {
			return fmt.Errorf("process function %d: %w", i, err)
		}

		changed, err := HasChanges()
		if err != nil {
			return fmt.Errorf("checking changes: %w", err)
		}

		if !changed {
			logrus.WithField("function", i).Info("Nothing changed, skip commit ...")
			continue
		}

		anyChange = true
		if err := gitCommit(o.GitName, o.GitEmail, msg, stdout, stderr, false); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}
	if !anyChange {
		logrus.Info("Nothing changed from all functions, skip PR ...")
		return nil
	}

	if err := gitPush(fmt.Sprintf("https://%s:%s@github.com/%s/%s.git", o.GitHubLogin, string(sa.GetTokenGenerator(o.GitHubToken)()), o.GitHubLogin, o.RemoteName), o.HeadBranchName, stdout, stderr, o.SkipPullRequest); err != nil {
		return fmt.Errorf("push changes to the remote branch: %w", err)
	}

	summary, body, err := prh.PRTitleBody()
	if err != nil {
		return fmt.Errorf("creating PR summary and body: %w", err)
	}
	if o.GitHubBaseBranch == "" {
		repo, err := gc.GetRepo(o.GitHubOrg, o.GitHubRepo)
		if err != nil {
			return fmt.Errorf("detect default remote branch for %s/%s: %w", o.GitHubOrg, o.GitHubRepo, err)
		}
		o.GitHubBaseBranch = repo.DefaultBranch
	}
	if err := updatePRWithLabels(gc, o.GitHubOrg, o.GitHubRepo, getAssignment(o.AssignTo, o.OncallAddress, o.OncallGroup), o.GitHubLogin, o.GitHubBaseBranch, o.HeadBranchName, updater.PreventMods, summary, body, o.Labels, o.SkipPullRequest); err != nil {
		return fmt.Errorf("to create the PR: %w", err)
	}
	return nil
}

func processGerrit(o *Options, prh PRHandler) error {
	var sa secret.Agent
	stdout := HideSecretsWriter{Delegate: os.Stdout, Censor: &sa}
	stderr := HideSecretsWriter{Delegate: os.Stderr, Censor: &sa}

	if err := Call(stdout, stderr, gitCmd, "config", "http.cookiefile", o.Gerrit.CookieFile); err != nil {
		return fmt.Errorf("unable to load cookiefile: %v", err)
	}
	if err := Call(stdout, stderr, gitCmd, "config", "user.name", o.Gerrit.Author); err != nil {
		return fmt.Errorf("unable to set username: %v", err)
	}
	if err := Call(stdout, stderr, gitCmd, "config", "user.email", o.Gerrit.Email); err != nil {
		return fmt.Errorf("unable to set password: %v", err)
	}
	if err := Call(stdout, stderr, gitCmd, "remote", "add", "upstream", o.Gerrit.HostRepo); err != nil {
		return fmt.Errorf("unable to add upstream remote: %v", err)
	}
	changeId, err := getChangeId(o.Gerrit.Author, o.Gerrit.AutobumpPRIdentifier, "")
	if err != nil {
		return fmt.Errorf("Failed to create CR: %w", err)
	}

	// Make change, commit and push
	for i, changeFunc := range prh.Changes() {
		msg, err := changeFunc()
		if err != nil {
			return fmt.Errorf("process function %d: %w", i, err)
		}

		changed, err := HasChanges()
		if err != nil {
			return fmt.Errorf("checking changes: %w", err)
		}

		if !changed {
			logrus.WithField("function", i).Info("Nothing changed, skip commit ...")
			continue
		}

		if err = gerritCommitandPush(msg, o.Gerrit.AutobumpPRIdentifier, changeId, nil, nil, stdout, stderr); err != nil {
			// If push because a closed PR already exists with this
			// change ID (the PR was abandoned). Hash the ID again and try one
			// more time.
			if !strings.Contains(err.Error(), "push some refs") || !strings.Contains(err.Error(), "closed") {
				return err
			}
			logrus.Warn("Error pushing CR due to already used ChangeID. PR may have been abandoned. Trying again with new ChangeID.")
			if changeId, err = getChangeId(o.Gerrit.Author, o.Gerrit.AutobumpPRIdentifier, changeId); err != nil {
				return err
			}
			if err := Call(stdout, stderr, gitCmd, "reset", "HEAD^"); err != nil {
				return fmt.Errorf("unable to call git reset: %v", err)
			}
			return gerritCommitandPush(msg, o.Gerrit.AutobumpPRIdentifier, changeId, nil, nil, stdout, stderr)
		}
	}
	return nil
}

func gerritCommitandPush(summary, autobumpId, changeId string, reviewers, cc []string, stdout, stderr io.Writer) error {
	msg := makeGerritCommit(summary, autobumpId, changeId)

	// TODO(mpherman): Add reviewers to CreateCR
	if err := createCR(msg, "master", changeId, reviewers, cc, stdout, stderr); err != nil {
		return fmt.Errorf("create CR: %w", err)
	}
	return nil
}

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("chdir to bazel workspace (%s): %w", bazelWorkspace, err)
		}
		return nil
	}
	cmd := exec.Command(gitCmd, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("get the repo's root directory: %w", err)
	}
	d := strings.TrimSpace(string(output))
	logrus.Infof("Changing working directory to %s...", d)
	return os.Chdir(d)
}

func Call(stdout, stderr io.Writer, cmd string, args ...string) error {
	(&logrus.Logger{
		Out:       stderr,
		Formatter: logrus.StandardLogger().Formatter,
		Hooks:     logrus.StandardLogger().Hooks,
		Level:     logrus.StandardLogger().Level,
	}).WithField("cmd", cmd).
		// The default formatting uses a space as separator, which is hard to read if an arg contains a space
		WithField("args", fmt.Sprintf("['%s']", strings.Join(args, "', '"))).
		Info("running command")

	c := exec.Command(cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}

type Censor interface {
	Censor(content []byte) []byte
}

type HideSecretsWriter struct {
	Delegate io.Writer
	Censor   Censor
}

func (w HideSecretsWriter) Write(content []byte) (int, error) {
	_, err := w.Delegate.Write(w.Censor.Censor(content))
	if err != nil {
		return 0, err
	}
	return len(content), nil
}

// UpdatePR updates with github client "gc" the PR of github repo org/repo
// with headBranch from "source" to "baseBranch"
// "images" contains the tag replacements that have been made which is returned from "updateReferences([]string{"."}, extraFiles)"
// "images" and "extraLineInPRBody" are used to generate commit summary and body of the PR
func UpdatePR(gc github.Client, org, repo string, extraLineInPRBody, login, baseBranch, headBranch string, allowMods bool, summary, body string) error {
	return updatePRWithLabels(gc, org, repo, extraLineInPRBody, login, baseBranch, headBranch, allowMods, summary, body, nil, false)
}
func updatePRWithLabels(gc github.Client, org, repo string, extraLineInPRBody, login, baseBranch, headBranch string, allowMods bool, summary, body string, labels []string, dryrun bool) error {
	return UpdatePullRequestWithLabels(gc, org, repo, summary, generatePRBody(body, extraLineInPRBody), login+":"+headBranch, baseBranch, headBranch, allowMods, labels, dryrun)
}

// UpdatePullRequest updates with github client "gc" the PR of github repo org/repo
// with "title" and "body" of PR matching author and headBranch from "source" to "baseBranch"
func UpdatePullRequest(gc github.Client, org, repo, title, body, source, baseBranch, headBranch string, allowMods bool, dryrun bool) error {
	return UpdatePullRequestWithLabels(gc, org, repo, title, body, source, baseBranch, headBranch, allowMods, nil, dryrun)
}

// UpdatePullRequestWithLabels updates with github client "gc" the PR of github repo org/repo
// with "title" and "body" of PR matching author and headBranch from "source" to "baseBranch" with labels
func UpdatePullRequestWithLabels(gc github.Client, org, repo, title, body, source, baseBranch,
	headBranch string, allowMods bool, labels []string, dryrun bool) error {
	logrus.Info("Creating or updating PR...")
	if dryrun {
		logrus.Info("[Dryrun] ensure PR with:")
		logrus.Info(org, repo, title, body, source, baseBranch, headBranch, allowMods, gc, labels, dryrun)
		return nil
	}
	n, err := updater.EnsurePRWithLabels(org, repo, title, body, source, baseBranch, headBranch, allowMods, gc, labels)
	if err != nil {
		return fmt.Errorf("ensure PR exists: %w", err)
	}
	logrus.Infof("PR %s/%s#%d will merge %s into %s: %s", org, repo, *n, source, baseBranch, title)
	return nil
}

// HasChanges checks if the current git repo contains any changes
func HasChanges() (bool, error) {
	args := []string{"status", "--porcelain"}
	logrus.WithField("cmd", gitCmd).WithField("args", args).Info("running command ...")
	combinedOutput, err := exec.Command(gitCmd, args...).CombinedOutput()
	if err != nil {
		logrus.WithField("cmd", gitCmd).Debugf("output is '%s'", string(combinedOutput))
		return false, fmt.Errorf("running command %s %s: %w", gitCmd, args, err)
	}
	return len(strings.TrimSuffix(string(combinedOutput), "\n")) > 0, nil
}

// MakeGitCommit runs a sequence of git commands to
// commit and push the changes the "remote" on "remoteBranch"
// "name" and "email" are used for git-commit command
// "images" contains the tag replacements that have been made which is returned from "updateReferences([]string{"."}, extraFiles)"
// "images" is used to generate commit message
func MakeGitCommit(remote, remoteBranch, name, email string, stdout, stderr io.Writer, summary string, dryrun bool) error {
	return GitCommitAndPush(remote, remoteBranch, name, email, summary, stdout, stderr, dryrun)
}

func makeGerritCommit(summary, commitTag, changeId string) string {
	return fmt.Sprintf("%s\n\n[%s]\n\nChange-Id: %s", summary, commitTag, changeId)
}

// GitCommitAndPush runs a sequence of git commands to commit.
// The "name", "email", and "message" are used for git-commit command
func GitCommitAndPush(remote, remoteBranch, name, email, message string, stdout, stderr io.Writer, dryrun bool) error {
	return GitCommitSignoffAndPush(remote, remoteBranch, name, email, message, stdout, stderr, false, dryrun)
}

// GitCommitSignoffAndPush runs a sequence of git commands to commit with optional signoff for the commit.
// The "name", "email", and "message" are used for git-commit command
func GitCommitSignoffAndPush(remote, remoteBranch, name, email, message string, stdout, stderr io.Writer, signoff bool, dryrun bool) error {
	logrus.Info("Making git commit...")

	if err := gitCommit(name, email, message, stdout, stderr, signoff); err != nil {
		return err
	}
	return gitPush(remote, remoteBranch, stdout, stderr, dryrun)
}
func gitCommit(name, email, message string, stdout, stderr io.Writer, signoff bool) error {
	if err := Call(stdout, stderr, gitCmd, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	commitArgs := []string{"commit", "-m", message}
	if name != "" && email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", name, email))
	}
	if signoff {
		commitArgs = append(commitArgs, "--signoff")
	}
	if err := Call(stdout, stderr, gitCmd, commitArgs...); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func gitPush(remote, remoteBranch string, stdout, stderr io.Writer, dryrun bool) error {
	if err := Call(stdout, stderr, gitCmd, "remote", "add", forkRemoteName, remote); err != nil {
		return fmt.Errorf("add remote: %w", err)
	}
	fetchStderr := &bytes.Buffer{}
	var remoteTreeRef string
	if err := Call(stdout, fetchStderr, gitCmd, "fetch", forkRemoteName, remoteBranch); err != nil {
		logrus.Info("fetchStderr is : ", fetchStderr.String())
		if !strings.Contains(strings.ToLower(fetchStderr.String()), fmt.Sprintf("couldn't find remote ref %s", remoteBranch)) {
			return fmt.Errorf("fetch from fork: %w", err)
		}
	} else {
		var err error
		remoteTreeRef, err = getTreeRef(stderr, fmt.Sprintf("refs/remotes/%s/%s", forkRemoteName, remoteBranch))
		if err != nil {
			return fmt.Errorf("get remote tree ref: %w", err)
		}
	}
	localTreeRef, err := getTreeRef(stderr, "HEAD")
	if err != nil {
		return fmt.Errorf("get local tree ref: %w", err)
	}

	if dryrun {
		logrus.Info("[Dryrun] Skip git push with: ")
		logrus.Info(forkRemoteName, remoteBranch, stdout, stderr, "")
		return nil
	}
	// Avoid doing metadata-only pushes that re-trigger tests and remove lgtm
	if localTreeRef != remoteTreeRef {
		if err := GitPush(forkRemoteName, remoteBranch, stdout, stderr, ""); err != nil {
			return err
		}
	} else {
		logrus.Info("Not pushing as up-to-date remote branch already exists")
	}
	return nil
}

// GitPush push the changes to the given remote and branch.
func GitPush(remote, remoteBranch string, stdout, stderr io.Writer, workingDir string) error {
	logrus.Info("Pushing to remote...")
	gc := GitCommand{
		baseCommand: gitCmd,
		args:        []string{"push", "-f", remote, fmt.Sprintf("HEAD:%s", remoteBranch)},
		workingDir:  workingDir,
	}
	if err := gc.Call(stdout, stderr); err != nil {
		return fmt.Errorf("%s: %w", gc.getCommand(), err)
	}
	return nil
}
func generatePRBody(body, assignment string) string {
	return body + assignment + "\n"
}

func getAssignment(assignTo, oncallAddress, oncallGroup string) string {
	if assignTo != "" {
		return "/cc @" + assignTo
	}
	if oncallAddress == "" {
		return ""
	}

	req, err := http.Get(oncallAddress)
	if err != nil {
		return fmt.Sprintf(errOncallMsgTempl, err)
	}
	defer req.Body.Close()
	if req.StatusCode != http.StatusOK {
		return fmt.Sprintf(errOncallMsgTempl,
			fmt.Sprintf("Error requesting oncall address: HTTP error %d: %q", req.StatusCode, req.Status))
	}
	oncall := struct {
		Oncall map[string]string `json:"Oncall"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&oncall); err != nil {
		return fmt.Sprintf(errOncallMsgTempl, err)
	}
	curtOncall, ok := oncall.Oncall[oncallGroup]
	if !ok {
		return fmt.Sprintf(errOncallMsgTempl, fmt.Sprintf("Oncall map doesn't contain group '%s'", oncallGroup))
	}
	if curtOncall != "" {
		return "/cc @" + curtOncall
	}
	return noOncallMsg
}

func getTreeRef(stderr io.Writer, refname string) (string, error) {
	revParseStdout := &bytes.Buffer{}
	if err := Call(revParseStdout, stderr, gitCmd, "rev-parse", refname+":"); err != nil {
		return "", fmt.Errorf("parse ref: %w", err)
	}
	fields := strings.Fields(revParseStdout.String())
	if n := len(fields); n < 1 {
		return "", errors.New("got no otput when trying to rev-parse")
	}
	return fields[0], nil
}

func buildPushRef(branch string, reviewers, cc []string) string {
	pushRef := fmt.Sprintf("HEAD:refs/for/%s", branch)
	var addedOptions []string
	for _, v := range reviewers {
		addedOptions = append(addedOptions, fmt.Sprintf("r=%s", v))
	}
	for _, v := range cc {
		addedOptions = append(addedOptions, fmt.Sprintf("cc=%s", v))
	}
	if len(addedOptions) > 0 {
		pushRef = fmt.Sprintf("%s%%%s", pushRef, strings.Join(addedOptions, ","))
	}
	return pushRef
}

func getDiff(prevCommit string) (string, error) {
	var diffBuf bytes.Buffer
	var errBuf bytes.Buffer
	if err := Call(&diffBuf, &errBuf, gitCmd, "diff", prevCommit); err != nil {
		return "", fmt.Errorf("diffing previous bump: %v -- %s", err, errBuf.String())
	}
	return diffBuf.String(), nil
}

func gerritNoOpChange(changeID string) (bool, error) {
	var garbageBuf bytes.Buffer
	var outBuf bytes.Buffer
	// Fetch current pending CRs
	if err := Call(&garbageBuf, &garbageBuf, gitCmd, "fetch", "upstream", "+refs/changes/*:refs/remotes/upstream/changes/*"); err != nil {
		return false, fmt.Errorf("unable to fetch upstream changes: %v -- \nOUTPUT: %s", err, garbageBuf.String())
	}
	// Get PR with same ChangeID for this bump
	if err := Call(&outBuf, &garbageBuf, gitCmd, "log", "--all", fmt.Sprintf("--grep=Change-Id: %s", changeID), "-1", "--format=%H"); err != nil {
		return false, fmt.Errorf("getting previous bump: %v", err)
	}
	prevCommit := strings.TrimSpace(outBuf.String())
	// No current CRs with cur ChangeID means this is not a noOp change
	if prevCommit == "" {
		return false, nil
	}
	diff, err := getDiff(prevCommit)
	if err != nil {
		return false, err
	}
	if diff == "" {
		return true, nil
	}
	return false, nil

}

func createCR(msg, branch, changeID string, reviewers, cc []string, stdout, stderr io.Writer) error {
	noOp, err := gerritNoOpChange(changeID)
	if err != nil {
		return fmt.Errorf("diffing previous bump: %v", err)
	}
	if noOp {
		logrus.Info("CR is a no-op change. Returning without pushing update")
		return nil
	}

	pushRef := buildPushRef(branch, reviewers, cc)
	if err := Call(stdout, stderr, gitCmd, "commit", "-a", "-v", "-m", msg); err != nil {
		return fmt.Errorf("unable to commit: %v", err)
	}
	if err := Call(stdout, stderr, gitCmd, "push", "upstream", pushRef); err != nil {
		return fmt.Errorf("unable to push: %v", err)
	}
	return nil
}

func getLastBumpCommit(gerritAuthor, commitTag string) (string, error) {
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	if err := Call(&outBuf, &errBuf, gitCmd, "log", fmt.Sprintf("--author=%s", gerritAuthor), fmt.Sprintf("--grep=%s", commitTag), "-1", "--format='%H'"); err != nil {
		return "", errors.New("running git command")
	}

	return outBuf.String(), nil
}

// getChangeId generates a change ID for the gerrit PR that is deterministic
// rather than being random as is normally preferable.
// In particular this chooses a change ID by hashing the last commit by the
// robot with a given string in the commit message (This string will be added to all autobump commit messages)
// if there is no commit by the robot with this commit tag, we assume that the job has never run, or that the robot/commit tag has changed
// in either case, the deterministic ID is generated by just hashing a string of the author + commit tag
func getChangeId(gerritAuthor, commitTag, startingID string) (string, error) {
	var id string
	if startingID == "" {
		lastBumpCommit, err := getLastBumpCommit(gerritAuthor, commitTag)
		if err != nil {
			return "", fmt.Errorf("Error getting change Id: %w", err)
		}
		if lastBumpCommit != "" {
			id = "I" + gitHash(lastBumpCommit)
		} else {
			// If it is the first time the autobumper has run a commit will not exist with the tag
			// create a deterministic tag by hashing the tag itself instead of the last commit.
			id = "I" + gitHash(gerritAuthor+commitTag)
		}
	} else {
		id = gitHash(startingID)
	}
	gitLog, err := getFullLog()
	if err != nil {
		return "", err
	}
	//While a commit on the base branch exists with this change ID...
	for strings.Contains(gitLog, id) {
		// Choose another ID by hashing the current ID.
		id = "I" + gitHash(id)
	}

	return id, nil
}

func getFullLog() (string, error) {
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	if err := Call(&outBuf, &errBuf, gitCmd, "log"); err != nil {
		return "", fmt.Errorf("unable to run git log: %w, %s", err, errBuf.String())
	}
	return outBuf.String(), nil
}

func gitHash(hashing string) string {
	h := sha1.New()
	io.WriteString(h, hashing)
	return fmt.Sprintf("%x", h.Sum(nil))
}
