/*
Copyright 2017 The Kubernetes Authors.

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

package mungers

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	githubapi "github.com/google/go-github/github"
)

// OwnerLabelMunger will label issues as specified in OWNERS files.
type OwnerLabelMunger struct {
	labeler fileLabeler
}

type fileLabeler interface {
	AllPossibleOwnerLabels() sets.String
	FindLabelsForPath(path string) sets.String
}

func init() {
	ownerLabel := &OwnerLabelMunger{}
	RegisterMungerOrDie(ownerLabel)
}

// Name is the name usable in --pr-mungers
func (b *OwnerLabelMunger) Name() string { return "owner-label" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *OwnerLabelMunger) RequiredFeatures() []string {
	return []string{features.RepoFeatureName}
}

// Initialize will initialize the munger
func (b *OwnerLabelMunger) Initialize(config *github.Config, features *features.Features) error {
	b.labeler = features.Repos
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *OwnerLabelMunger) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (b *OwnerLabelMunger) RegisterOptions(opts *options.Options) sets.String { return nil }

func (b *OwnerLabelMunger) getLabels(files []*githubapi.CommitFile) sets.String {
	labels := sets.String{}
	for _, file := range files {
		if file == nil {
			continue
		}
		if file.Changes == nil || *file.Changes == 0 {
			continue
		}
		fileLabels := b.labeler.FindLabelsForPath(*file.Filename)
		labels = labels.Union(fileLabels)
	}
	return labels
}

// Munge is the workhorse the will actually make updates to the PR
func (b *OwnerLabelMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	needsLabels := b.getLabels(files)

	// TODO: make sure no other munger considers itself to own a label in
	// AllPossibleOwnerLabels, and then pass that so that this will remove
	// as well as add labels.
	SyncLabels(needsLabels, needsLabels, obj)
	// SyncLabels(b.labeler.AllPossibleOwnerLabels(), needsLabels, obj)
}
