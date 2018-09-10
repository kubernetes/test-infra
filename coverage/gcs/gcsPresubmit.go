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

// Package gcs prototypes uploading resource (go test coverage profile) to GCS
// if enable debug, then the reading from GCS feature would be run as well
package gcs

import (
	"path"
	"strconv"

	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
)

//ArtifactsDirNameOnGcs is the name used by the artifacts folder, on gcs
const ArtifactsDirNameOnGcs = "artifacts"

//PresubmitBuild is the sub-type of GcsBuild with data specific to pre-submit workflow
type PresubmitBuild struct {
	GcsBuild
	Artifacts     GcsArtifacts
	PostSubmitJob string
}

//PreSubmit is the common sub-type of GithubPr and PresubmitBuild
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

//MakeGcsArtifacts converts a LocalArtifacts to a GcsArtifacts
func (p *PreSubmit) MakeGcsArtifacts(localArts artifacts.LocalArtifacts) *GcsArtifacts {
	localArts.SetDirectory(p.relDirOfArtifacts())
	res := newGcsArtifacts(p.Ctx, p.StorageClient, p.Bucket, localArts.Artifacts)
	return res
}

func (p *PreSubmit) urlLineCov() (result string) {
	return path.Join(p.urlArtifactsDir(), artifacts.LineCovFileName)
}

//UrlGcsLineCovLinkWithMarker composes the line coverage link
func (p *PreSubmit) UrlGcsLineCovLinkWithMarker(section int) (result string) {
	return "https://" + p.urlLineCov() + "#file" + strconv.Itoa(section)
}
