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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	imagebumper "k8s.io/test-infra/experiment/image-bumper/bumper"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const (
	prowPrefix      = "gcr.io/k8s-prow/"
	boskosPrefix    = "gcr.io/k8s-staging-boskos/"
	testImagePrefix = "gcr.io/k8s-testimages/"
	prowRepo        = "https://github.com/kubernetes/test-infra"
	testImageRepo   = prowRepo
	boskosRepo      = "https://github.com/kubernetes-sigs/boskos"
	forkRemoteName  = "bumper-fork-remote"

	latestVersion          = "latest"
	upstreamVersion        = "upstream"
	upstreamStagingVersion = "upstream-staging"
	tagVersion             = "vYYYYMMDD-deadbeef"

	upstreamURLBase            = "https://raw.githubusercontent.com/kubernetes/test-infra/master"
	prowRefConfigFile          = "config/prow/cluster/deck_deployment.yaml"
	prowStagingRefConfigFile   = "config/prow-staging/cluster/deck_deployment.yaml"
	boskosRefConfigFile        = "config/prow/cluster/boskos.yaml"
	boskosStagingRefConfigFile = "config/prow-staging/cluster/boskos.yaml"

	errOncallMsgTempl = "An error occurred while finding an assignee: `%s`.\nFalling back to Blunderbuss."
	noOncallMsg       = "Nobody is currently oncall, so falling back to Blunderbuss."

	gitCmd = "git"
)

var (
	tagRegexp    = regexp.MustCompile("v[0-9]{8}-[a-f0-9]{6,9}")
	imageMatcher = regexp.MustCompile(`(?s)^.+image:.+:(v[a-zA-Z0-9_.-]+)`)
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
			return fmt.Errorf("error getting file info for %q", fn)
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
	GitHubOrg     string
	GitHubRepo    string
	GitHubLogin   string
	GitHubToken   string
	GitName       string
	GitEmail      string
	RemoteBranch  string
	OncallAddress string

	BumpProwImages   bool
	BumpBoskosImages bool
	BumpTestImages   bool
	TargetVersion    string

	IncludedConfigPaths fileArrayFlag
	ExcludedConfigPaths fileArrayFlag
	ExtraFiles          fileArrayFlag

	SkipPullRequest bool
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
	if !o.SkipPullRequest && o.GitHubToken == "" {
		return fmt.Errorf("--github-token is mandatory when --skip-pull-request is false")
	}
	if !o.SkipPullRequest && (o.GitHubOrg == "" || o.GitHubRepo == "") {
		return fmt.Errorf("--github-org and --github-repo are mandatory when --skip-pull-request is false")
	}
	if !o.SkipPullRequest && o.RemoteBranch == "" {
		return fmt.Errorf("--remote-branch cannot be empty when --skip-pull-request is false")
	}
	if (o.GitEmail == "") != (o.GitName == "") {
		return fmt.Errorf("--git-name and --git-email must be specified together")
	}

	if o.TargetVersion != latestVersion && o.TargetVersion != upstreamVersion &&
		o.TargetVersion != upstreamStagingVersion && !tagRegexp.MatchString(o.TargetVersion) {
		logrus.Warnf("Warning: --target-version is not one of %v so it might not work properly.",
			[]string{latestVersion, upstreamVersion, upstreamStagingVersion, tagVersion})
	}
	if !o.BumpProwImages && !o.BumpBoskosImages && !o.BumpTestImages {
		return fmt.Errorf("at least one of --bump-prow-images, --bump-boskos-images and --bump-test-images must be specified")
	}
	if (o.BumpProwImages || o.BumpBoskosImages) && o.BumpTestImages && o.TargetVersion != latestVersion {
		return fmt.Errorf("--target-version must be latest if you want to bump both prow/boskos and test images")
	}
	if o.BumpTestImages && (o.TargetVersion == upstreamVersion || o.TargetVersion == upstreamStagingVersion) {
		return fmt.Errorf("%q and %q versions can only be specified to bump prow/boskos images", upstreamVersion, upstreamStagingVersion)
	}

	if len(o.IncludedConfigPaths) == 0 {
		return fmt.Errorf("--include-config-paths is mandatory")
	}

	return nil
}

// Run is the entrypoint which will update Prow config files based on the provided options.
func Run(o *Options) error {
	if err := validateOptions(o); err != nil {
		return fmt.Errorf("error validating options: %w", err)
	}

	if err := cdToRootDir(); err != nil {
		return fmt.Errorf("failed to change to root dir: %w", err)
	}

	images, err := UpdateReferences(
		o.BumpProwImages, o.BumpBoskosImages, o.BumpTestImages, o.TargetVersion,
		o.IncludedConfigPaths, o.ExcludedConfigPaths, o.ExtraFiles)
	if err != nil {
		return fmt.Errorf("failed to update image references: %w", err)
	}

	changed, err := HasChanges()
	if err != nil {
		return fmt.Errorf("error occurred when checking changes: %w", err)
	}

	if !changed {
		logrus.Info("no images updated, exiting ...")
		return nil
	}

	if o.SkipPullRequest {
		logrus.Debugf("--skip-pull-request is set to true, won't create a pull request.")
	} else {
		var sa secret.Agent
		if err := sa.Start([]string{o.GitHubToken}); err != nil {
			return fmt.Errorf("failed to start secrets agent: %w", err)
		}

		gc := github.NewClient(sa.GetTokenGenerator(o.GitHubToken), sa.Censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

		if o.GitHubLogin == "" || o.GitName == "" || o.GitEmail == "" {
			user, err := gc.BotUser()
			if err != nil {
				return fmt.Errorf("failed to get the user data for the provided GH token: %w", err)
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

		remoteBranch := "autobump"
		stdout := HideSecretsWriter{Delegate: os.Stdout, Censor: &sa}
		stderr := HideSecretsWriter{Delegate: os.Stderr, Censor: &sa}
		if err := MakeGitCommit(fmt.Sprintf("git@github.com:%s/test-infra.git", o.GitHubLogin), remoteBranch, o.GitName, o.GitEmail, images, stdout, stderr); err != nil {
			return fmt.Errorf("failed to push changes to the remote branch: %w", err)
		}

		if err := UpdatePR(gc, o.GitHubOrg, o.GitHubRepo, images, getAssignment(o.OncallAddress), "Update prow to", o.GitHubLogin+":"+remoteBranch, "master", updater.PreventMods); err != nil {
			return fmt.Errorf("failed to create the PR: %w", err)
		}
	}

	return nil
}

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %w", bazelWorkspace, err)
		}
		return nil
	}
	cmd := exec.Command(gitCmd, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get the repo's root directory: %w", err)
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
// with "matchTitle" from "source" to "branch"
// "images" contains the tag replacements that have been made which is returned from "updateReferences([]string{"."}, extraFiles)"
// "images" and "extraLineInPRBody" are used to generate commit summary and body of the PR
func UpdatePR(gc github.Client, org, repo string, images map[string]string, extraLineInPRBody string, matchTitle, source, branch string, allowMods bool) error {
	summary, err := makeCommitSummary(images)
	if err != nil {
		return err
	}
	return UpdatePullRequest(gc, org, repo, summary, generatePRBody(images, extraLineInPRBody), matchTitle, source, branch, allowMods)
}

// UpdatePullRequest updates with github client "gc" the PR of github repo org/repo
// with "title" and "body" of PR matching "matchTitle" from "source" to "branch"
func UpdatePullRequest(gc github.Client, org, repo, title, body, matchTitle, source, branch string, allowMods bool) error {
	return UpdatePullRequestWithLabels(gc, org, repo, title, body, matchTitle, source, branch, allowMods, nil)
}

func UpdatePullRequestWithLabels(gc github.Client, org, repo, title, body, matchTitle, source, branch string, allowMods bool, labels []string) error {
	logrus.Info("Creating or updating PR...")
	n, err := updater.EnsurePRWithLabels(org, repo, title, body, source, branch, matchTitle, allowMods, gc, labels)
	if err != nil {
		return fmt.Errorf("failed to ensure PR exists: %w", err)
	}

	logrus.Infof("PR %s/%s#%d will merge %s into %s: %s", org, repo, *n, source, branch, title)
	return nil
}

// updateReferences update the references of prow-images and/or boskos-images and/or testimages
// in the files in any of "subfolders" of the includeConfigPaths but not in excludeConfigPaths
// if the file is a yaml file (*.yaml) or extraFiles[file]=true
func UpdateReferences(bumpProwImages, bumpBoskosImages bool, bumpTestImages bool, targetVersion string,
	includeConfigPaths []string, excludeConfigPaths []string, extraFiles []string) (map[string]string, error) {
	logrus.Info("Bumping image references...")
	filters := make([]string, 0)
	if bumpProwImages {
		filters = append(filters, prowPrefix)
	}
	if bumpBoskosImages {
		filters = append(filters, boskosPrefix)
	}
	if bumpTestImages {
		filters = append(filters, testImagePrefix)
	}
	filterRegexp := regexp.MustCompile(strings.Join(filters, "|"))

	imageBumperCli := imagebumper.NewClient()
	return updateReferences(imageBumperCli, filterRegexp, targetVersion, includeConfigPaths, excludeConfigPaths, extraFiles)
}

type imageBumper interface {
	FindLatestTag(imageHost, imageName, currentTag string) (string, error)
	UpdateFile(tagPicker func(imageHost, imageName, currentTag string) (string, error), path string, imageFilter *regexp.Regexp) error
	GetReplacements() map[string]string
}

func updateReferences(imageBumperCli imageBumper, filterRegexp *regexp.Regexp, targetVersion string,
	includeConfigPaths []string, excludeConfigPaths []string, extraFiles []string) (map[string]string, error) {
	var tagPicker func(string, string, string) (string, error)
	var err error
	switch targetVersion {
	case latestVersion:
		tagPicker = imageBumperCli.FindLatestTag
	case upstreamVersion, upstreamStagingVersion:
		tagPicker, err = upstreamImageVersionResolver(targetVersion, parseUpstreamImageVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image version: %w", targetVersion, err)
		}
	default:
		tagPicker = func(imageHost, imageName, currentTag string) (string, error) { return targetVersion, nil }
	}

	updateFile := func(name string) error {
		logrus.Infof("Updating file %s", name)
		if err := imageBumperCli.UpdateFile(tagPicker, name, filterRegexp); err != nil {
			return fmt.Errorf("failed to update the file: %w", err)
		}
		return nil
	}
	updateYAMLFile := func(name string) error {
		if strings.HasSuffix(name, ".yaml") && !isUnderPath(name, excludeConfigPaths) {
			return updateFile(name)
		}
		return nil
	}

	// Updated all .yaml files under the included config paths but not under excluded config paths.
	for _, path := range includeConfigPaths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to get the file info for %q", path)
		}
		if info.IsDir() {
			err := filepath.Walk(path, func(subpath string, info os.FileInfo, err error) error {
				return updateYAMLFile(subpath)
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update yaml files under %q: %w", path, err)
			}
		} else {
			if err := updateYAMLFile(path); err != nil {
				return nil, fmt.Errorf("failed to update the yaml file %q: %w", path, err)
			}
		}
	}

	// Update the extra files in any case.
	for _, file := range extraFiles {
		if err := updateFile(file); err != nil {
			return nil, fmt.Errorf("failed to update the extra file %q: %w", file, err)
		}
	}

	return imageBumperCli.GetReplacements(), nil
}

func upstreamImageVersionResolver(
	upstreamVersionType string, parse func(upstreamAddress string) (string, error),
) (func(imageHost, imageName, currentTag string) (string, error), error) {
	prowUpstreamAddress, boskosUpstreamAddress, err := upstreamConfigFileAddresses(upstreamVersionType)
	if err != nil {
		return nil, err
	}

	prowVersion, err := parse(prowUpstreamAddress)
	if err != nil {
		return nil, fmt.Errorf("error resolving the upstream Prow version from %q: %w", prowUpstreamAddress, err)
	}
	boskosVersion, err := parse(boskosUpstreamAddress)
	if err != nil {
		return nil, fmt.Errorf("error resolving the upstream Boskos version from %q: %w", boskosUpstreamAddress, err)
	}
	return func(imageHost, imageName, currentTag string) (string, error) {
		imageFullPath := imageHost + "/" + imageName
		if strings.HasPrefix(imageFullPath, prowPrefix) {
			return prowVersion, nil
		} else if strings.HasPrefix(imageFullPath, boskosPrefix) {
			return boskosVersion, nil
		} else {
			return currentTag, nil
		}
	}, nil
}

// upstreamConfigFileAddresses returns the upstream configuration file addresses for parsing the image version.
func upstreamConfigFileAddresses(upstreamVersionType string) (prowUpstreamAddress, boskosUpstreamAddress string, err error) {
	if upstreamVersionType == upstreamVersion {
		prowUpstreamAddress = upstreamURLBase + "/" + prowRefConfigFile
		boskosUpstreamAddress = upstreamURLBase + "/" + boskosRefConfigFile
	} else if upstreamVersionType == upstreamStagingVersion {
		prowUpstreamAddress = upstreamURLBase + "/" + prowStagingRefConfigFile
		boskosUpstreamAddress = upstreamURLBase + "/" + boskosStagingRefConfigFile
	} else {
		return "", "", fmt.Errorf("unsupported upstream version type: %s, must be one of %v",
			upstreamVersionType, []string{upstreamVersion, upstreamStagingVersion})
	}

	return
}

func parseUpstreamImageVersion(upstreamAddress string) (string, error) {
	resp, err := http.Get(upstreamAddress)
	if err != nil {
		return "", fmt.Errorf("error sending GET request to %q: %w", upstreamAddress, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error %d (%q) fetching upstream config file", resp.StatusCode, resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading the response body: %w", err)
	}
	res := imageMatcher.FindStringSubmatch(string(body))
	if len(res) < 2 {
		return "", fmt.Errorf("the image tag is malformatted: %v", res)
	}
	return res[1], nil
}

func isUnderPath(name string, paths []string) bool {
	for _, p := range paths {
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

type versionTargetMatching struct {
	version string
	targets []string
}

func getNewProwVersion(images map[string]string) (string, error) {
	found := map[string][]string{}
	for k, v := range images {
		if strings.HasPrefix(k, prowPrefix) {
			found[v] = append(found[v], k)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		for version := range found {
			return version, nil
		}
	}

	// Need to sort to avoid flaky tests
	var result []versionTargetMatching
	for version, targets := range found {
		sort.Slice(targets, func(i, j int) bool {
			return targets[i] < targets[j]
		})
		result = append(result, versionTargetMatching{version: version, targets: targets})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].version < result[j].version
	})
	return "", fmt.Errorf(
		"Expected a consistent version for all %q images, but found multiple: %v",
		prowPrefix,
		result)
}

// HasChanges checks if the current git repo contains any changes
func HasChanges() (bool, error) {
	args := []string{"status", "--porcelain"}
	logrus.WithField("cmd", gitCmd).WithField("args", args).Info("running command ...")
	combinedOutput, err := exec.Command(gitCmd, args...).CombinedOutput()
	if err != nil {
		logrus.WithField("cmd", gitCmd).Debugf("output is '%s'", string(combinedOutput))
		return false, fmt.Errorf("error running command %s %s: %w", gitCmd, args, err)
	}
	return len(strings.TrimSuffix(string(combinedOutput), "\n")) > 0, nil
}

func makeCommitSummary(images map[string]string) (string, error) {
	version, err := getNewProwVersion(images)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Update prow to %s, and other images as necessary.", version), nil
}

// MakeGitCommit runs a sequence of git commands to
// commit and push the changes the "remote" on "remoteBranch"
// "name" and "email" are used for git-commit command
// "images" contains the tag replacements that have been made which is returned from "updateReferences([]string{"."}, extraFiles)"
// "images" is used to generate commit message
func MakeGitCommit(remote, remoteBranch, name, email string, images map[string]string, stdout, stderr io.Writer) error {
	summary, err := makeCommitSummary(images)
	if err != nil {
		return err
	}
	return GitCommitAndPush(remote, remoteBranch, name, email, summary, stdout, stderr)
}

// GitCommitAndPush runs a sequence of git commands to commit.
// The "name", "email", and "message" are used for git-commit command
func GitCommitAndPush(remote, remoteBranch, name, email, message string, stdout, stderr io.Writer) error {
	logrus.Info("Making git commit...")

	if err := Call(stdout, stderr, gitCmd, "add", "-A"); err != nil {
		return fmt.Errorf("failed to git add: %w", err)
	}
	commitArgs := []string{"commit", "-m", message}
	if name != "" && email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", name, email))
	}
	if err := Call(stdout, stderr, gitCmd, commitArgs...); err != nil {
		return fmt.Errorf("failed to git commit: %w", err)
	}
	if err := Call(stdout, stderr, gitCmd, "remote", "add", forkRemoteName, remote); err != nil {
		return fmt.Errorf("failed to add remote: %w", err)
	}
	fetchStderr := &bytes.Buffer{}
	var remoteTreeRef string
	if err := Call(stdout, fetchStderr, gitCmd, "fetch", forkRemoteName, remoteBranch); err != nil {
		if !strings.Contains(strings.ToLower(fetchStderr.String()), fmt.Sprintf("couldn't find remote ref %s", remoteBranch)) {
			return fmt.Errorf("failed to fetch from fork: %w", err)
		}
	} else {
		var err error
		remoteTreeRef, err = getTreeRef(stderr, fmt.Sprintf("refs/remotes/%s/%s", forkRemoteName, remoteBranch))
		if err != nil {
			return fmt.Errorf("failed to get remote tree ref: %w", err)
		}
	}

	localTreeRef, err := getTreeRef(stderr, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get local tree ref: %w", err)
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
		return fmt.Errorf("failed to %s: %w", gc.getCommand(), err)
	}
	return nil
}

// RunAndCommitIfNeeded makes a commit in the workingDir if there are
// any changes resulting from the command execution. Returns true if a commit is made
func RunAndCommitIfNeeded(stdout, stderr io.Writer, author, cmd string, args []string, workingDir string) (bool, error) {
	fullCommand := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))

	logrus.Infof("Running: %s", fullCommand)
	if err := Call(stdout, stderr, cmd, args...); err != nil {
		return false, fmt.Errorf("failed to run %s: %w", fullCommand, err)
	}

	changed, err := HasChanges()
	if err != nil {
		return false, fmt.Errorf("error occurred when checking changes: %w", err)
	}

	if !changed {
		logrus.WithField("command", fullCommand).Info("No changes to commit")
		return false, nil
	}
	gc := GitCommand{
		baseCommand: gitCmd,
		args:        []string{"add", "."},
		workingDir:  workingDir,
	}
	if err := gc.Call(stdout, stderr); err != nil {
		return false, fmt.Errorf("failed to %s: %w", gc.getCommand(), err)
	}
	gc = GitCommand{
		baseCommand: gitCmd,
		args:        []string{"commit", "-m", fullCommand, "--author", author},
		workingDir:  workingDir,
	}
	if err := gc.Call(stdout, stderr); err != nil {
		return false, fmt.Errorf("failed to %s: %w", gc.getCommand(), err)
	}

	return true, nil
}

func tagFromName(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func componentFromName(name string) string {
	s := strings.SplitN(strings.Split(name, ":")[0], "/", 3)
	return s[len(s)-1]
}

func formatTagDate(d string) string {
	if len(d) != 8 {
		return d
	}
	// &#x2011; = U+2011 NON-BREAKING HYPHEN, to prevent line wraps.
	return fmt.Sprintf("%s&#x2011;%s&#x2011;%s", d[0:4], d[4:6], d[6:8])
}

func generateSummary(name, repo, prefix string, summarise bool, images map[string]string) string {
	type delta struct {
		oldCommit string
		newCommit string
		oldDate   string
		newDate   string
		variant   string
		component string
	}
	versions := map[string][]delta{}
	for image, newTag := range images {
		if !strings.HasPrefix(image, prefix) {
			continue
		}
		if strings.HasSuffix(image, ":"+newTag) {
			continue
		}
		oldDate, oldCommit, oldVariant := imagebumper.DeconstructTag(tagFromName(image))
		newDate, newCommit, _ := imagebumper.DeconstructTag(newTag)
		k := oldCommit + ":" + newCommit
		d := delta{
			oldCommit: oldCommit,
			newCommit: newCommit,
			oldDate:   oldDate,
			newDate:   newDate,
			variant:   oldVariant,
			component: componentFromName(image),
		}
		versions[k] = append(versions[k], d)
	}

	switch {
	case len(versions) == 0:
		return fmt.Sprintf("No %s changes.", name)
	case len(versions) == 1 && summarise:
		for k, v := range versions {
			s := strings.Split(k, ":")
			return fmt.Sprintf("%s changes: %s/compare/%s...%s (%s → %s)", name, repo, s[0], s[1], formatTagDate(v[0].oldDate), formatTagDate(v[0].newDate))
		}
	default:
		changes := make([]string, 0, len(versions))
		for k, v := range versions {
			s := strings.Split(k, ":")
			names := make([]string, 0, len(v))
			for _, d := range v {
				names = append(names, d.component+d.variant)
			}
			sort.Strings(names)
			changes = append(changes, fmt.Sprintf("%s/compare/%s...%s | %s&nbsp;&#x2192;&nbsp;%s | %s",
				repo, s[0], s[1], formatTagDate(v[0].oldDate), formatTagDate(v[0].newDate), strings.Join(names, ", ")))
		}
		sort.Slice(changes, func(i, j int) bool { return strings.Split(changes[i], "|")[1] < strings.Split(changes[j], "|")[1] })
		return fmt.Sprintf("Multiple distinct %s changes:\n\nCommits | Dates | Images\n--- | --- | ---\n%s\n", name, strings.Join(changes, "\n"))
	}
	panic("unreachable!")
}

func generatePRBody(images map[string]string, assignment string) string {
	prowSummary := generateSummary("Prow", prowRepo, prowPrefix, true, images)
	testImagesSummary := generateSummary("test-image", testImageRepo, testImagePrefix, false, images)
	boskosSummary := generateSummary("Boskos", boskosRepo, boskosPrefix, false, images)
	return prowSummary + "\n\n" + testImagesSummary + "\n\n" + boskosSummary + "\n\n" + assignment + "\n"
}

func getAssignment(oncallAddress string) string {
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
		Oncall struct {
			TestInfra string `json:"testinfra"`
		} `json:"Oncall"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&oncall); err != nil {
		return fmt.Sprintf(errOncallMsgTempl, err)
	}
	curtOncall := oncall.Oncall.TestInfra
	if curtOncall != "" {
		return "/cc @" + curtOncall
	}
	return noOncallMsg
}

func getTreeRef(stderr io.Writer, refname string) (string, error) {
	revParseStdout := &bytes.Buffer{}
	if err := Call(revParseStdout, stderr, gitCmd, "rev-parse", refname+":"); err != nil {
		return "", fmt.Errorf("failed to parse ref: %w", err)
	}
	fields := strings.Fields(revParseStdout.String())
	if n := len(fields); n < 1 {
		return "", errors.New("got no otput when trying to rev-parse")
	}
	return fields[0], nil
}
