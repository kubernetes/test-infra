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
	forkRemoteName = "bumper-fork-remote"

	latestVersion          = "latest"
	upstreamVersion        = "upstream"
	upstreamStagingVersion = "upstream-staging"
	tagVersion             = "vYYYYMMDD-deadbeef"
	defaultUpstreamURLBase = "https://raw.githubusercontent.com/kubernetes/test-infra/master"

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
	//The GitHub org name where the autobump PR will be created. Must not be empty when SkipPullRequest is false.
	GitHubOrg string `yaml:"gitHubOrg"`
	//The GitHub repo name where the autobump PR will be created. Must not be empty when SkipPullRequest is false.
	GitHubRepo string `yaml:"gitHubRepo"`
	//The GitHub username to use. If not specified, uses values from the user associated with the access token.
	GitHubLogin string `yaml:"gitHubLogin"`
	//The path to the GitHub token file.
	GitHubToken string `yaml:"gitHubToken"`
	//The name to use on the git commit. Requires GitEmail. If not specified, uses values from the user associated with the access token
	GitName string `yaml:"gitName"`
	//"The email to use on the git commit. Requires GitName. If not specified, uses values from the user associated with the access token.
	GitEmail string `yaml:"gitEmail"`
	//The oncall address where we can get the JSON file that stores the current oncall information.
	OncallAddress string `yaml:"onCallAddress"`
	//Whether to skip creating the pull request for this bump.
	SkipPullRequest bool `yaml:"skipPullRequest"`
	//The URL where upstream images are located. Must not be empty if Target Version is "upstream" or "upstreamStaging"
	UpstreamURLBase string `yaml:"upstreamURLBase"`
	//The config paths to be included in this bump, in which only .yaml files will be considered. By default all files are included.
	IncludedConfigPaths []string `yaml:"includedConfigPaths"`
	//The config paths to be excluded in this bump, in which only .yaml files will be considered.
	ExcludedConfigPaths []string `yaml:"excludedConfigPaths"`
	//The extra non-yaml file to be considered in this bump.
	ExtraFiles []string `yaml:"extraFiles"`
	//The target version to bump images version to, which can be one of latest, upstream, upstream-staging and vYYYYMMDD-deadbeef.
	TargetVersion string `yaml:"targetVersion"`
	//The name used in the address when creating remote. Format will be git@github.com:{GitLogin}/{RemoteName}.git
	RemoteName string `yaml:"remoteName"`
	//List of prefixes that the autobumped is looking for, and other information needed to bump them
	Prefixes []Prefix `yaml:"prefixes"`
}

// Prefix is the information needed for each prefix being bumped.
type Prefix struct {
	//Name of the tool being bumped
	Name string `yaml:"name"`
	//The image prefix that the autobumper should look for
	Prefix string `yaml:"prefix"`
	//File that is looked at when bumping to match upstream
	RefConfigFile string `yaml:"refConfigFile"`
	//File that is looked at when bumping to match upstreamStaging
	StagingRefConfigFile string `yaml:"stagingRefConfigFile"`
	//Repo used when generating pull request
	Repo string `yaml:"repo"`
	//Whether or not the format of the PR summary for this prefix should be summarised.
	Summarise bool `yaml:"summarise"`
	//Whether the prefix tags should be consistent after the bump
	ConsistentImages bool `yaml:"consistentImages"`
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
	if len(o.Prefixes) == 0 {
		return fmt.Errorf("Must have at least one Prefix specified")
	}
	if !o.SkipPullRequest && o.GitHubToken == "" {
		return fmt.Errorf("gitHubToken is mandatory when skipPullRequest is false or unspecified")
	}
	if (o.GitEmail == "") != (o.GitName == "") {
		return fmt.Errorf("gitName and gitEmail must be specified together")
	}
	if !o.SkipPullRequest && (o.GitHubOrg == "" || o.GitHubRepo == "") {
		return fmt.Errorf("gitHubOrg and gitHubRepo are mandatory when skipPullRequest is false or unspecified")
	}
	if !o.SkipPullRequest && (o.RemoteName == "") {
		return fmt.Errorf("remoteName is mandatory when skipPullRequest is false or unspecified")
	}
	if len(o.IncludedConfigPaths) == 0 {
		return fmt.Errorf("includedConfigPaths is mandatory")
	}
	if o.TargetVersion != latestVersion && o.TargetVersion != upstreamVersion &&
		o.TargetVersion != upstreamStagingVersion && !tagRegexp.MatchString(o.TargetVersion) {
		logrus.Warnf("Warning: targetVersion is not one of %v so it might not work properly.",
			[]string{latestVersion, upstreamVersion, upstreamStagingVersion, tagVersion})
	}
	if o.TargetVersion == upstreamVersion {
		for _, prefix := range o.Prefixes {
			if prefix.RefConfigFile == "" {
				return fmt.Errorf("targetVersion can't be %q without refConfigFile for each prefix. %q is missing one", upstreamVersion, prefix.Name)
			}
		}
	}
	if o.TargetVersion == upstreamStagingVersion {
		for _, prefix := range o.Prefixes {
			if prefix.StagingRefConfigFile == "" {
				return fmt.Errorf("targetVersion can't be %q without stagingRefConfigFile for each prefix. %q is missing one", upstreamStagingVersion, prefix.Name)
			}
		}
	}
	if o.TargetVersion == upstreamVersion || o.TargetVersion == upstreamStagingVersion && o.UpstreamURLBase == "" {
		o.UpstreamURLBase = defaultUpstreamURLBase
		logrus.Warnf("targetVersion can't be 'upstream' or 'upstreamStaging` without upstreamURLBase set. Default upstreamURLBase is %q", defaultUpstreamURLBase)
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

	images, err := UpdateReferences(o)
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

	versions, err := getVersionsAndCheckConsistency(o.Prefixes, images)
	if err != nil {
		return err
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
		if err := MakeGitCommit(fmt.Sprintf("git@github.com:%s/%s.git", o.GitHubLogin, o.RemoteName), remoteBranch, o.GitName, o.GitEmail, o.Prefixes, stdout, stderr, versions); err != nil {
			return fmt.Errorf("failed to push changes to the remote branch: %w", err)
		}

		if err := UpdatePR(gc, o.GitHubOrg, o.GitHubRepo, images, getAssignment(o.OncallAddress), "Update prow to", o.GitHubLogin+":"+remoteBranch, "master", updater.PreventMods, o.Prefixes, versions); err != nil {
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
func UpdatePR(gc github.Client, org, repo string, images map[string]string, extraLineInPRBody, matchTitle, source, branch string, allowMods bool, prefixes []Prefix, versions map[string][]string) error {
	summary := makeCommitSummary(prefixes, versions)
	return UpdatePullRequest(gc, org, repo, summary, generatePRBody(images, extraLineInPRBody, prefixes), matchTitle, source, branch, allowMods)
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

func getAllPrefixes(prefixList []Prefix) (res []string) {
	for _, prefix := range prefixList {
		res = append(res, prefix.Prefix)
	}
	return res
}

// UpdateReferences update the references of prow-images and/or boskos-images and/or testimages
// in the files in any of "subfolders" of the includeConfigPaths but not in excludeConfigPaths
// if the file is a yaml file (*.yaml) or extraFiles[file]=true
func UpdateReferences(o *Options) (map[string]string, error) {
	logrus.Info("Bumping image references...")
	filterRegexp := regexp.MustCompile(strings.Join(getAllPrefixes(o.Prefixes), "|"))
	imageBumperCli := imagebumper.NewClient()
	return updateReferences(imageBumperCli, filterRegexp, o)
}

type imageBumper interface {
	FindLatestTag(imageHost, imageName, currentTag string) (string, error)
	UpdateFile(tagPicker func(imageHost, imageName, currentTag string) (string, error), path string, imageFilter *regexp.Regexp) error
	GetReplacements() map[string]string
	AddToCache(image, newTag string)
}

func updateReferences(imageBumperCli imageBumper, filterRegexp *regexp.Regexp, o *Options) (map[string]string, error) {
	var tagPicker func(string, string, string) (string, error)
	var err error
	switch o.TargetVersion {
	case latestVersion:
		tagPicker = imageBumperCli.FindLatestTag
	case upstreamVersion, upstreamStagingVersion:
		tagPicker, err = upstreamImageVersionResolver(o, o.TargetVersion, parseUpstreamImageVersion, imageBumperCli)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image version: %w", o.TargetVersion, err)
		}
	default:
		tagPicker = func(imageHost, imageName, currentTag string) (string, error) { return o.TargetVersion, nil }
	}

	updateFile := func(name string) error {
		logrus.Infof("Updating file %s", name)
		if err := imageBumperCli.UpdateFile(tagPicker, name, filterRegexp); err != nil {
			return fmt.Errorf("failed to update the file: %w", err)
		}
		return nil
	}
	updateYAMLFile := func(name string) error {
		if strings.HasSuffix(name, ".yaml") && !isUnderPath(name, o.ExcludedConfigPaths) {
			return updateFile(name)
		}
		return nil
	}

	// Updated all .yaml files under the included config paths but not under excluded config paths.
	for _, path := range o.IncludedConfigPaths {
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
	for _, file := range o.ExtraFiles {
		if err := updateFile(file); err != nil {
			return nil, fmt.Errorf("failed to update the extra file %q: %w", file, err)
		}
	}

	return imageBumperCli.GetReplacements(), nil
}

func upstreamImageVersionResolver(
	o *Options, upstreamVersionType string, parse func(upstreamAddress string) (string, error), imageBumperCli imageBumper) (func(imageHost, imageName, currentTag string) (string, error), error) {
	upstreamVersions, err := upstreamConfigVersions(upstreamVersionType, o, parse)
	if err != nil {
		return nil, err
	}

	return func(imageHost, imageName, currentTag string) (string, error) {
		imageFullPath := imageHost + "/" + imageName + "/" + currentTag
		for prefix, version := range upstreamVersions {
			if strings.HasPrefix(imageFullPath, prefix) {
				imageBumperCli.AddToCache(imageFullPath, version)
				return version, nil
			}
		}
		return currentTag, nil
	}, nil
}

func upstreamConfigVersions(upstreamVersionType string, o *Options, parse func(upstreamAddress string) (string, error)) (versions map[string]string, err error) {
	versions = make(map[string]string)
	var upstreamAddress string
	for _, prefix := range o.Prefixes {
		if upstreamVersionType == upstreamVersion {
			upstreamAddress = o.UpstreamURLBase + "/" + prefix.RefConfigFile
		} else if upstreamVersionType == upstreamStagingVersion {
			upstreamAddress = o.UpstreamURLBase + "/" + prefix.StagingRefConfigFile
		} else {
			return nil, fmt.Errorf("unsupported upstream version type: %s, must be one of %v",
				upstreamVersionType, []string{upstreamVersion, upstreamStagingVersion})
		}
		version, err := parse(upstreamAddress)
		if err != nil {
			return nil, err
		}
		versions[prefix.Prefix] = version
	}

	return versions, nil
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

func getVersionsAndCheckConsistency(prefixes []Prefix, images map[string]string) (map[string][]string, error) {
	// Key is tag, value is full image.
	versions := map[string][]string{}
	for _, prefix := range prefixes {
		newVersions := 0
		for k, v := range images {
			if strings.HasPrefix(k, prefix.Prefix) {
				if _, ok := versions[v]; !ok {
					newVersions++
				}
				versions[v] = append(versions[v], k)
				if prefix.ConsistentImages && newVersions > 1 {
					return nil, fmt.Errorf("%q was supposed to be bumped consistently but was not", prefix.Name)
				}
			}
		}
	}
	return versions, nil
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

func getPrefixesString(prefixes []Prefix) string {
	return strings.Join(getAllPrefixes(prefixes), ", ")
}

func makeCommitSummary(prefixes []Prefix, versions map[string][]string) string {
	if len(versions) == 0 {
		return fmt.Sprintf("Update %s images as necessary", getPrefixesString(prefixes))
	}
	inconsistentBumps := ""
	consistentBumps := "Update"
	for _, prefix := range prefixes {
		if !prefix.ConsistentImages {
			inconsistentBumps = fmt.Sprintf("%s, %s", prefix.Prefix, inconsistentBumps)
		} else {
			for tag, imageList := range versions {
				//Should not be possible for tag to be in map with empty imageList
				if strings.HasPrefix(imageList[0], prefix.Prefix) {
					consistentBumps = fmt.Sprintf("%s %s to %s,", consistentBumps, prefix.Prefix, tag)
					break
				}
			}
		}
	}
	if inconsistentBumps != "" {
		return fmt.Sprintf("%s and %s as needed", consistentBumps, inconsistentBumps)
	}
	return consistentBumps

}

// MakeGitCommit runs a sequence of git commands to
// commit and push the changes the "remote" on "remoteBranch"
// "name" and "email" are used for git-commit command
// "images" contains the tag replacements that have been made which is returned from "updateReferences([]string{"."}, extraFiles)"
// "images" is used to generate commit message
func MakeGitCommit(remote, remoteBranch, name, email string, prefixes []Prefix, stdout, stderr io.Writer, versions map[string][]string) error {
	summary := makeCommitSummary(prefixes, versions)
	return GitCommitAndPush(remote, remoteBranch, name, email, summary, stdout, stderr)
}

// GitCommitAndPush runs a sequence of git commands to commit.
// The "name", "email", and "message" are used for git-commit command
func GitCommitAndPush(remote, remoteBranch, name, email, message string, stdout, stderr io.Writer) error {
	return GitCommitSignoffAndPush(remote, remoteBranch, name, email, message, stdout, stderr, false)
}

// GitCommitSignoffAndPush runs a sequence of git commands to commit with optional signoff for the commit.
// The "name", "email", and "message" are used for git-commit command
func GitCommitSignoffAndPush(remote, remoteBranch, name, email, message string, stdout, stderr io.Writer, signoff bool) error {
	logrus.Info("Making git commit...")

	if err := Call(stdout, stderr, gitCmd, "add", "-A"); err != nil {
		return fmt.Errorf("failed to git add: %w", err)
	}
	commitArgs := []string{"commit", "-m", message}
	if name != "" && email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", name, email))
	}
	if signoff {
		commitArgs = append(commitArgs, "--signoff")
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

func generatePRBody(images map[string]string, assignment string, prefixes []Prefix) (body string) {
	body = ""
	for _, prefix := range prefixes {
		body = body + generateSummary(prefix.Name, prefix.Repo, prefix.Prefix, prefix.Summarise, images) + "\n\n"
	}
	return body + assignment + "\n"
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
