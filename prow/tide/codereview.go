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
	"encoding/json"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
)

// CodeReviewForDeck contains superset of data from CodeReviewCommon, it's meant
// to be consumed by deck only.
//
// Tide serves Pool data to deck via http request inside cluster, which could
// contain many PullRequests, sending over full PullRequest struct could be very
// expensive in some cases.
type CodeReviewForDeck struct {
	Title      string
	Number     int
	HeadRefOID string
	Mergeable  string
}

func FromCodeReviewCommonToCodeReviewForDeck(crc *CodeReviewCommon) *CodeReviewForDeck {
	if crc == nil {
		return nil
	}
	return &CodeReviewForDeck{
		Title:      crc.Title,
		Number:     crc.Number,
		HeadRefOID: crc.HeadRefOID,
		Mergeable:  crc.Mergeable,
	}
}

// MinCodeReviewCommon can be casted into full CodeReviewCommon, which will
// result in json marshal/unmarshal overrides.
//
// This should be used only right before serialization, and for now it's
// consumed only by Deck.
type MinCodeReviewCommon CodeReviewCommon

// MarshalJSON marshals MinCodeReviewCommon into CodeReviewForDeck
func (m *MinCodeReviewCommon) MarshalJSON() ([]byte, error) {
	min := &CodeReviewForDeck{
		Title:      m.Title,
		Number:     m.Number,
		HeadRefOID: m.HeadRefOID,
		Mergeable:  m.Mergeable,
	}
	return json.Marshal(min)
}

// UnmarshalJSON overrides unmarshal function, the marshalled bytes should only
// be used by Typescript for now
func (m *MinCodeReviewCommon) UnmarshalJSON(b []byte) error {
	return errors.New("this is not implemented")
}

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

	Title string
	Body  string
	// AuthorLogin is the author login from the fork on GitHub, this will be the
	// author login from Gerrit.
	AuthorLogin   string
	UpdatedAtTime time.Time

	Mergeable    string
	CanBeRebased bool

	GitHub *PullRequest
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

// GitHubLabels returns labels struct for GitHub, using this function is almost
// equivalent to `if isGitHub() {// then do that}`.
//
// This is useful for determining the merging strategy.
func (crc *CodeReviewCommon) GitHubLabels() *Labels {
	if crc.GitHub == nil {
		return nil
	}
	return &crc.GitHub.Labels
}

// GitHubCommits returns Commits struct from GitHub.
//
// This is used by checking status context to determine whether the PR is ready
// for merge or not.
func (crc *CodeReviewCommon) GitHubCommits() *Commits {
	if crc.GitHub == nil {
		return nil
	}
	return &crc.GitHub.Commits
}

// CodeReviewCommonFromPullRequest derives CodeReviewCommon struct from GitHub
// PullRequest struct, by extracting shared fields among different code review
// providers.
func CodeReviewCommonFromPullRequest(pr *PullRequest) *CodeReviewCommon {
	if pr == nil {
		return nil
	}
	// Make a copy
	prCopy := *pr
	crc := &CodeReviewCommon{
		NameWithOwner: string(pr.Repository.NameWithOwner),
		Number:        int(pr.Number),
		Org:           string(pr.Repository.Owner.Login),
		Repo:          string(pr.Repository.Name),
		BaseRefPrefix: string(pr.BaseRef.Prefix),
		BaseRefName:   string(pr.BaseRef.Name),
		HeadRefName:   string(pr.HeadRefName),
		HeadRefOID:    string(pr.HeadRefOID),
		Title:         string(pr.Title),
		Body:          string(pr.Body),
		AuthorLogin:   string(pr.Author.Login),
		Mergeable:     string(pr.Mergeable),
		CanBeRebased:  bool(pr.CanBeRebased),
		UpdatedAtTime: pr.UpdatedAt.Time,

		GitHub: &prCopy,
	}

	return crc
}
