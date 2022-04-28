/*
Copyright 2022 The Kubernetes Authors.

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

package tide

import (
	"time"

	"github.com/sirupsen/logrus"
)

type CodeReviewCommon struct {
	// NameWithOwner is from graphql.NameWithOwner, <org>/<repo>
	NameWithOwner string
	// The number of PR
	Number int
	Org    string
	Repo   string
	// BaseRefPrefix gets prefix of ref, such as /refs/head, /refs/tags
	BaseRefPrefix string
	BaseRefName   string
	HeadRefName   string
	HeadRefOID    string

	Title       string
	Body        string
	AuthorLogin string
	// Labels gets labels on the PR.
	// TODO(chaodaiG): labels might mean something different on gerrit, consider
	// how to name this nicely.
	Labels        []string
	IsMergeable   bool
	UpdatedAtTime time.Time

	Mergeable    string
	CanBeRebased bool
	LogFields    logrus.Fields

	// Likely GitHub only
	Milestone      *Milestone
	Commits        Commits
	ReviewDecision string
	RepoWithOwner  string
}

func (crc *CodeReviewCommon) logFields() logrus.Fields {
	return logrus.Fields{
		"org":    crc.Org,
		"repo":   crc.Repo,
		"pr":     crc.Number,
		"branch": crc.BaseRefName,
		"sha":    crc.HeadRefOID,
	}
}

func CodeReviewCommonFromPullRequest(pr *PullRequest) *CodeReviewCommon {
	crc := &CodeReviewCommon{
		NameWithOwner:  string(pr.Repository.NameWithOwner),
		Number:         int(pr.Number),
		Org:            string(pr.Repository.Owner.Login),
		Repo:           string(pr.Repository.Name),
		BaseRefPrefix:  string(pr.BaseRef.Prefix),
		BaseRefName:    string(pr.BaseRef.Name),
		HeadRefName:    string(pr.HeadRefName),
		HeadRefOID:     string(pr.HeadRefOID),
		Title:          string(pr.Title),
		Body:           string(pr.Body),
		AuthorLogin:    string(pr.Author.Login),
		Mergeable:      string(pr.Mergeable),
		CanBeRebased:   bool(pr.CanBeRebased),
		UpdatedAtTime:  pr.UpdatedAt.Time,
		Commits:        pr.Commits,
		ReviewDecision: string(pr.ReviewDecision),
		RepoWithOwner:  string(pr.Repository.NameWithOwner),
		Milestone:      pr.Milestone,
	}

	for _, label := range pr.Labels.Nodes {
		crc.Labels = append(crc.Labels, string(label.Name))
	}

	return crc
}
