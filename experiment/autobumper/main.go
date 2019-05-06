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
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/experiment/image-bumper/bumper"
	"k8s.io/test-infra/prow/github"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const prowPrefix = "gcr.io/k8s-prow/"
const testImagePrefix = "gcr.io/k8s-testimages/"
const prowRepo = "https://github.com/kubernetes/test-infra"
const testImageRepo = prowRepo
const oncallAddress = "https://storage.googleapis.com/kubernetes-jenkins/oncall.json"
const githubOrg = "kubernetes"
const githubRepo = "test-infra"

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %v", bazelWorkspace, err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.Chdir(strings.TrimSpace(string(output)))
}

type options struct {
	githubLogin string
	githubToken string
	gitName string
	gitEmail string
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.githubLogin, "github-login", "", "The GitHub username to use.")
	flag.StringVar(&o.githubToken, "github-token", "", "The path to the GitHub token file.")
	flag.StringVar(&o.gitName, "git-name", "", "The name to use on the git commit. If not specified, uses the system default.")
	flag.StringVar(&o.gitEmail, "git-email", "", "The email to use on the git commit. If not specified, uses the system default.")
	flag.Parse()
	return o
}

func updatePR(gc *github.Client, org, repo, title, body, matchTitle, source, branch string) error {
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
	filter := regexp.MustCompile(`gcr\.io/(?:k8s-prow|k8s-testimages)`)

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".yaml") {
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
	return fmt.Sprintf("Updated prow to %s, and other images as necessary.", getNewProwVersion(images))
}

func makeGitCommit(user string, images map[string]string) error {
	if err := exec.Command("git", "add", "-A").Run(); err != nil {
		return fmt.Errorf("failed to git add: %v", err)
	}
	message := makeCommitSummary(images)
	if err := exec.Command("git", "commit", "-m", message).Run(); err != nil {
		return fmt.Errorf("failed to git commit: %v", err)
	}
	if err := exec.Command("git", "push", "-f", fmt.Sprintf("git@github.com:%s/test-infra.git", user), "HEAD:autobump").Run(); err != nil {
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
	s := strings.Split("/", strings.Split(name, ":")[0])
	return s[len(s)-1]
}

func commitFromTag(tag string) string {
	s := strings.Split(tag, "-")
	return s[len(s)-1]
}

func getSomeSummary(name, repo, prefix string, images map[string]string) string {
	prowVersions := map[string][]string{}
	for image, newTag := range images {
		if !strings.HasPrefix(image, prefix) {
			continue
		}
		if strings.HasSuffix(image, ":" +newTag) {
			continue
		}
		oldTag := tagFromName(image)
		k := oldTag + ":" + newTag
		prowVersions[k] = append(prowVersions[k], componentFromName(image))
	}

	switch len(prowVersions) {
	case 0:
		return fmt.Sprintf("No %s changes.", name)
	case 1:
		for k := range prowVersions {
			s := strings.Split(k, ":")
			return fmt.Sprintf("%s changes: %s/compare/%s...%s", name, repo, commitFromTag(s[0]), commitFromTag(s[1]))
		}
	default:
		changes := make([]string, 0, len(prowVersions))
		for k, v := range prowVersions {
			s := strings.Split(k, ":")
			changes = append(changes, fmt.Sprintf("* %s/compare/%s...%s: %s", repo, commitFromTag(s[0]), commitFromTag(s[1]), strings.Join(v, ", ")))
		}
		return fmt.Sprintf("Multiple distinct %s changes:\n\n%s", name, strings.Join(changes, "\n"))
	}
	panic("unreachable!")
}

func getOncaller() (string, error) {
	req, err := http.Get(oncallAddress)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()
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
	prowSummary := getSomeSummary("Prow", prowRepo, prowPrefix, images)
	testImagesSummary := getSomeSummary("Test image", testImageRepo, testImagePrefix, images)
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

	jamesBond := &secret.Agent{}
	if err := jamesBond.Start([]string{o.githubToken}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}

	gc := github.NewClient(jamesBond.GetTokenGenerator(o.githubToken), github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

	if err := cdToRootDir(); err != nil {
		logrus.WithError(err).Fatal("Failed to change to root dir")
	}
	images, err := updateReferences()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to update references.")
	}

	if err := makeGitCommit(o.githubLogin, images); err != nil {
		logrus.WithError(err).Fatal("Failed to push changes.")
	}

	if err := updatePR(gc, githubOrg, githubRepo, makeCommitSummary(images), generatePRBody(images), "Updated prow to", o.githubLogin+":autobump", "master"); err != nil {
		logrus.WithError(err).Fatal("PR creation failed.")
	}
}
