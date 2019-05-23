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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/experiment/image-bumper/bumper"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const (
	prowPrefix      = "gcr.io/k8s-prow/"
	testImagePrefix = "gcr.io/k8s-testimages/"
	prowRepo        = "https://github.com/kubernetes/test-infra"
	testImageRepo   = prowRepo
	oncallAddress   = "https://storage.googleapis.com/kubernetes-jenkins/oncall.json"
	githubOrg       = "kubernetes"
	githubRepo      = "test-infra"
)

var extraFiles = map[string]bool{
	"experiment/generate_tests.py": true,
}

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %v", bazelWorkspace, err)
		}
		return nil
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	d := strings.TrimSpace(string(output))
	logrus.Infof("Changing working directory to %s...", d)
	return os.Chdir(d)
}

type options struct {
	githubLogin string
	githubToken string
	gitName     string
	gitEmail    string
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.githubLogin, "github-login", "", "The GitHub username to use.")
	flag.StringVar(&o.githubToken, "github-token", "", "The path to the GitHub token file.")
	flag.StringVar(&o.gitName, "git-name", "", "The name to use on the git commit. Requires --git-email. If not specified, uses the system default.")
	flag.StringVar(&o.gitEmail, "git-email", "", "The email to use on the git commit. Requires --git-name. If not specified, uses the system default.")
	flag.Parse()
	return o
}

func validateOptions(o options) error {
	if o.githubLogin == "" {
		return fmt.Errorf("--github-login is mandatory")
	}
	if o.githubToken == "" {
		return fmt.Errorf("--github-token is mandatory")
	}
	if (o.gitEmail == "") != (o.gitName == "") {
		return fmt.Errorf("--git-name and --git-email must be specified together")
	}
	return nil
}

func call(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func updatePR(gc github.Client, org, repo, title, body, matchTitle, source, branch string) error {
	logrus.Info("Creating PR...")
	n, err := updater.UpdatePR(org, repo, title, body, matchTitle, gc)
	if err != nil {
		return fmt.Errorf("failed to update %d: %v", n, err)
	}
	if n == nil {
		pr, err := gc.CreatePullRequest(org, repo, title, body, source, branch, true)
		if err != nil {
			return fmt.Errorf("failed to create PR: %v", err)
		}
		n = &pr
	}

	logrus.Infof("PR %s/%s#%d will merge %s into %s: %s", org, repo, *n, source, branch, title)
	return nil
}

func updateReferences() (map[string]string, error) {
	logrus.Info("Bumping image references...")
	filter := regexp.MustCompile(prowPrefix + "|" + testImagePrefix)

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".yaml") || extraFiles[path] {
			if err := bumper.UpdateFile(path, filter); err != nil {
				logrus.WithError(err).Errorf("Failed to update path %s.", path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return bumper.GetReplacements(), nil
}

func getNewProwVersion(images map[string]string) string {
	for k, v := range images {
		if strings.HasPrefix(k, prowPrefix) {
			return v
		}
	}
	return ""
}

func makeCommitSummary(images map[string]string) string {
	return fmt.Sprintf("Update prow to %s, and other images as necessary.", getNewProwVersion(images))
}

func updateConfig() error {
	// Try to regenerate security job configs which use an explicit podutils image config
	// TODO(krzyzacy): workaround before we resolve https://github.com/kubernetes/test-infra/issues/9783
	logrus.Info("Updating generated config...")
	return call("bazel", "run", "//hack:update-config")
}

func makeGitCommit(user, name, email string, images map[string]string) error {
	logrus.Info("Making git commit...")
	if err := call("git", "add", "-A"); err != nil {
		return fmt.Errorf("failed to git add: %v", err)
	}
	message := makeCommitSummary(images)
	commitArgs := []string{"commit", "-m", message}
	if name != "" && email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", name, email))
	}
	if err := call("git", commitArgs...); err != nil {
		return fmt.Errorf("failed to git commit: %v", err)
	}
	logrus.Info("Pushing to remote...")
	if err := call("git", "push", "-f", fmt.Sprintf("git@github.com:%s/test-infra.git", user), "HEAD:autobump"); err != nil {
		return fmt.Errorf("failed to git push: %v", err)
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
	s := strings.Split(strings.Split(name, ":")[0], "/")
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
		oldDate, oldCommit, oldVariant := bumper.DeconstructTag(tagFromName(image))
		newDate, newCommit, _ := bumper.DeconstructTag(newTag)
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

func getOncaller() (string, error) {
	req, err := http.Get(oncallAddress)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()
	if req.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error %d (%q) fetching current oncaller", req.StatusCode, req.Status)
	}
	oncall := struct {
		Oncall struct {
			TestInfra string `json:"testinfra"`
		} `json:"Oncall"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&oncall); err != nil {
		return "", err
	}
	return oncall.Oncall.TestInfra, nil
}

func generatePRBody(images map[string]string) string {
	prowSummary := generateSummary("Prow", prowRepo, prowPrefix, true, images)
	testImagesSummary := generateSummary("test-image", testImageRepo, testImagePrefix, false, images)
	oncaller, err := getOncaller()

	var assignment string
	if err == nil {
		if oncaller != "" {
			assignment = "/cc @" + oncaller
		} else {
			assignment = "Nobody is currently oncall, so falling back to Blunderbuss."
		}
	} else {
		assignment = fmt.Sprintf("An error occurred while finding an assignee: `%s`.\nFalling back to Blunderbuss.", err)
	}
	body := prowSummary + "\n\n" + testImagesSummary + "\n\n" + assignment + "\n"
	return body
}

func main() {
	o := parseOptions()
	if err := validateOptions(o); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}

	sa := &secret.Agent{}
	if err := sa.Start([]string{o.githubToken}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}

	gc := github.NewClient(sa.GetTokenGenerator(o.githubToken), github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

	if err := cdToRootDir(); err != nil {
		logrus.WithError(err).Fatal("Failed to change to root dir")
	}
	images, err := updateReferences()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to update references.")
	}
	if err := updateConfig(); err != nil {
		logrus.WithError(err).Fatal("Failed to update generated config.")
	}

	if err := makeGitCommit(o.githubLogin, o.gitName, o.gitEmail, images); err != nil {
		logrus.WithError(err).Fatal("Failed to push changes.")
	}

	if err := updatePR(gc, githubOrg, githubRepo, makeCommitSummary(images), generatePRBody(images), "Update prow to", o.githubLogin+":autobump", "master"); err != nil {
		logrus.WithError(err).Fatal("PR creation failed.")
	}
}
