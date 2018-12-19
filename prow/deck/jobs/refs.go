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

package jobs

import (
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

// RefData is a wrapper for information derived from a job's refs
type RefData struct {
	Repo           string   `json:"repo"`
	Refs           string   `json:"refs"`
	BaseRef        string   `json:"base_ref"`
	BaseSHA        string   `json:"base_sha"`
	PullSHA        string   `json:"pull_sha"`
	Number         int      `json:"number"`
	Author         string   `json:"author"`
	RepoLink       string   `json:"repo_link"`
	PullLink       string   `json:"pull_link"`
	PullCommitLink string   `json:"pull_commit_link"`
	PushCommitLink string   `json:"push_commit_link"`
	AuthorLink     string   `json:"author_link"`
	PRRefs         []int    `json:"pr_refs"`
	PRRefLinks     []string `json:"pr_ref_links"`
}

func hasGerritMetadata(job kube.ProwJob) bool {
	return job.ObjectMeta.Annotations[client.GerritID] != "" &&
		job.ObjectMeta.Annotations[client.GerritInstance] != "" &&
		job.ObjectMeta.Labels[client.GerritRevision] != ""
}

func getRefData(j kube.ProwJob) RefData {
	if hasGerritMetadata(j) {
		return gerritRefData(j)
	}
	return githubRefData(j)
}

func githubRefData(job kube.ProwJob) RefData {
	const github = "https://github.com"
	ref := RefData{
		Repo:    fmt.Sprintf("%s/%s", job.Spec.Refs.Org, job.Spec.Refs.Repo),
		Refs:    job.Spec.Refs.String(),
		BaseRef: job.Spec.Refs.BaseRef,
		BaseSHA: job.Spec.Refs.BaseSHA,
	}
	for _, pull := range job.Spec.Refs.Pulls {
		ref.PRRefs = append(ref.PRRefs, pull.Number)
		link := fmt.Sprintf("%s/%s/pull/%d", github, ref.Repo, pull.Number)
		ref.PRRefLinks = append(ref.PRRefLinks, link)
	}

	ref.RepoLink = fmt.Sprintf("%s/%s", github, ref.Repo)
	ref.PushCommitLink = fmt.Sprintf("%s/%s/commit/%s", github, ref.Repo, ref.BaseSHA)
	if len(job.Spec.Refs.Pulls) == 1 {
		ref.Number = job.Spec.Refs.Pulls[0].Number
		ref.Author = job.Spec.Refs.Pulls[0].Author
		ref.PullSHA = job.Spec.Refs.Pulls[0].SHA

		ref.AuthorLink = fmt.Sprintf("%s/%s", github, ref.Author)
		ref.PullLink = fmt.Sprintf("%s/%s/pull/%d", github, ref.Repo, ref.Number)
		ref.PullCommitLink = fmt.Sprintf("%s/commits/%s", ref.PullLink, ref.PullSHA)
	}
	return ref
}

func gerritRefData(job kube.ProwJob) RefData {
	reviewHost := job.ObjectMeta.Annotations[client.GerritInstance]
	parts := strings.SplitN(reviewHost, ".", 2)
	codeHost := strings.TrimSuffix(parts[0], "-review")
	if len(parts) > 1 {
		codeHost += "." + parts[1]
	}

	ref := RefData{
		Repo:    job.Spec.Refs.Repo,
		Refs:    job.Spec.Refs.String(),
		BaseRef: job.Spec.Refs.BaseRef,
		BaseSHA: job.Spec.Refs.BaseSHA,
	}
	ref.RepoLink = fmt.Sprintf("%s/%s", codeHost, ref.Repo)
	ref.PushCommitLink = fmt.Sprintf("%s/%s/+/%s", codeHost, ref.Repo, ref.BaseSHA)

	if len(job.Spec.Refs.Pulls) == 1 {
		ref.Number = job.Spec.Refs.Pulls[0].Number
		ref.Author = job.Spec.Refs.Pulls[0].Author
		ref.PullSHA = job.Spec.Refs.Pulls[0].SHA

		ref.AuthorLink = fmt.Sprintf("%s/q/%s", reviewHost, ref.Author)
		ref.PullLink = fmt.Sprintf("%s/c/%s/+/%d", reviewHost, ref.Repo, ref.Number)
		ref.PullCommitLink = fmt.Sprintf("%s/%s/+/%s", codeHost, ref.Repo, ref.PullSHA)
	}
	return ref
}
