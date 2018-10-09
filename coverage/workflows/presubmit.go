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

package workflows

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/calc"
	"k8s.io/test-infra/coverage/gcs"
	"k8s.io/test-infra/coverage/githubUtil"
	"k8s.io/test-infra/coverage/io"
	"k8s.io/test-infra/coverage/line"
)

// RunPresubmit performs all actions to be performed for presubmit workflow only
func RunPresubmit(p *gcs.PreSubmit, arts *artifacts.LocalArtifacts) (isCoverageLow bool) {
	logrus.Info("starting PreSubmit.RunPresubmit(...)")
	coverageThresholdInt := p.CovThreshold

	isLocalRun := os.Getenv("JOB_TYPE") == "local-presubmit"

	var concernedFiles *map[string]bool
	if isLocalRun {
		concernedFiles = &map[string]bool{}
	} else {
		concernedFiles = githubUtil.GetConcernedFiles(&p.GithubPr, "")
		if len(*concernedFiles) == 0 {
			logrus.Infof("List of concerned committed files is empty, " +
				"don't need to run coverage profile in presubmit\n")
			return false
		}
	}

	gNew := calc.CovList(arts.ProfileReader(), arts.KeyProfileCreator(), !isLocalRun,
		concernedFiles, coverageThresholdInt)
	line.CreateLineCovFile(arts)
	line.GenerateLineCovLinks(p, gNew)

	base := gcs.NewPostSubmit(p.Ctx, p.StorageClient, p.Bucket,
		p.PostSubmitJob, gcs.ArtifactsDirNameOnGcs, arts.ProfileName())

	gBase := calc.CovList(base.ProfileReader(), nil, false, concernedFiles, p.CovThreshold)
	changes := calc.NewGroupChanges(gBase, gNew)

	postContent, isEmpty, isCoverageLow := changes.ContentForGithubPost(concernedFiles)

	io.Write(&postContent, arts.Directory(), "bot-post")

	if isLocalRun {
		fmt.Printf("Content to post:\n%s\n", postContent)
	} else if !isEmpty {
		p.GithubPr.CleanAndPostComment(postContent)
	}

	logrus.Info("completed PreSubmit.RunPresubmit(...)")
	return
}
