package main

import (
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/calc"
	"k8s.io/test-infra/coverage/gcs"
	"k8s.io/test-infra/coverage/githubUtil"
	"k8s.io/test-infra/coverage/io"
	"k8s.io/test-infra/coverage/line"
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
