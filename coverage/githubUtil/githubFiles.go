/*
Copyright 2018 The Kubernetes Authors.

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

package githubUtil

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/github"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/coverage/git"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
	"k8s.io/test-infra/coverage/logUtil"
)

// return corresponding source file path of given path (abc_test.go -> abc.go)
func sourceFilePath(path string) string {
	if strings.HasSuffix(path, "_test.go") {
		return strings.TrimSuffix(path, "_test.go") + ".go"
	}
	return path
}

// GetConcernedFiles gets the list of files in a commit, excluding those to be ignored by coverage
func GetConcernedFiles(data *githubPR.GithubPr, filePathPrefix string) *map[string]bool {
	listOptions := &github.ListOptions{Page: 1}

	fmt.Println()
	logrus.Infof("GetConcernedFiles(...) started\n")

	commitFiles, _, err := data.GithubClient.PullRequests.ListFiles(data.Ctx, data.RepoOwner, data.RepoName,
		data.Pr, listOptions)
	if err != nil {
		logUtil.LogFatalf("error running c.PullRequests.ListFiles("+
			"Ctx, repoOwner=%s, RepoName=%s, pullNum=%v, listOptions): %v\n",
			data.RepoOwner, data.RepoName, data.Pr, err)
		return nil
	}

	fileNames := make(map[string]bool)
	for i, commitFile := range commitFiles {
		filePath := path.Join(filePathPrefix, sourceFilePath(*commitFile.Filename))
		isFileConcerned := !git.IsCoverageSkipped(filePath)
		logrus.Infof("github file #%d: %s, concerned=%v\n", i, filePath, isFileConcerned)
		fileNames[filePath] = isFileConcerned
	}

	logrus.Infof("GetConcernedFiles(...) completed\n\n")
	return &fileNames
}
