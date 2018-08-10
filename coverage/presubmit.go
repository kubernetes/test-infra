package main

import (
	"github.com/kubernetes/test-infra/coverage/artifacts"
	"github.com/kubernetes/test-infra/coverage/calc"
	"github.com/kubernetes/test-infra/coverage/gcs"
	"github.com/kubernetes/test-infra/coverage/githubUtil"
	"github.com/kubernetes/test-infra/coverage/io"
	"github.com/kubernetes/test-infra/coverage/line"
	"log"
)

func RunPresubmit(p *gcs.PreSubmit, arts *artifacts.LocalArtifacts) (isCoverageLow bool) {
	log.Println("starting PreSubmit.RunPresubmit(...)")
	coverageThresholdInt := p.CovThreshold

	concernedFiles := githubUtil.GetConcernedFiles(&p.GithubPr, "")

	if len(*concernedFiles) == 0 {
		log.Printf("List of concerned committed files is empty, " +
			"don't need to run coverage profile in presubmit\n")
		return false
	}

	gNew := calc.CovList(arts.ProfileReader(), arts.KeyProfileCreator(),
		concernedFiles, coverageThresholdInt)
	line.CreateLineCovFile(arts)
	line.GenerateLineCovLinks(p, gNew)

	base := gcs.NewPostSubmit(p.Ctx, p.StorageClient, p.Bucket,
		p.PostSubmitJob, gcs.ArtifactsDirNameOnGcs, arts.ProfileName())
	gBase := calc.CovList(base.ProfileReader(), nil, concernedFiles, p.CovThreshold)
	changes := calc.NewGroupChanges(gBase, gNew)

	postContent, isEmpty, isCoverageLow := changes.ContentForGithubPost(concernedFiles)

	io.Write(&postContent, arts.Directory(), "bot-post")

	if !isEmpty {
		p.GithubPr.CleanAndPostComment(postContent)
	}

	log.Println("completed PreSubmit.RunPresubmit(...)")
	return
}
