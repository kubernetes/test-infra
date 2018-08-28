// Package gcs prototypes uploading resource (go test coverage profile) to GCS
// if enable debug, then the reading from GCS feature would be run as well
package gcs

import (
	"path"
	"strconv"

	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
)

const ArtifactsDirNameOnGcs = "artifacts"

type PresubmitBuild struct {
	GcsBuild
	Artifacts     GcsArtifacts
	PostSubmitJob string
}

type PreSubmit struct {
	githubPR.GithubPr
	PresubmitBuild
}

func (p *PreSubmit) relDirOfJob() (result string) {
	return path.Join("pr-logs", "pull", p.RepoOwner+"_"+p.RepoName,
		p.PrStr(),
		p.Job)
}

func (p *PreSubmit) relDirOfBuild() (result string) {
	return path.Join(p.relDirOfJob(), p.BuildStr())
}

func (p *PreSubmit) relDirOfArtifacts() (result string) {
	return path.Join(p.relDirOfBuild(), ArtifactsDirNameOnGcs)
}

func (p *PreSubmit) urlArtifactsDir() (result string) {
	return path.Join(gcsUrlHost, p.Bucket, p.relDirOfArtifacts())
}

func (p *PreSubmit) MakeGcsArtifacts(localArts artifacts.LocalArtifacts) *GcsArtifacts {
	localArts.SetDirectory(p.relDirOfArtifacts())
	res := newGcsArtifacts(p.Ctx, p.StorageClient, p.Bucket, localArts.Artifacts)
	return res
}

func (p *PreSubmit) urlLineCov() (result string) {
	return path.Join(p.urlArtifactsDir(), artifacts.LineCovFileName)
}

func (p *PreSubmit) UrlGcsLineCovLinkWithMarker(section int) (result string) {
	return "https://" + p.urlLineCov() + "#file" + strconv.Itoa(section)
}
