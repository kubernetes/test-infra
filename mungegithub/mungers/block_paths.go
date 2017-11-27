/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"os"
	"regexp"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	blockedPathsLabel = "do-not-merge/blocked-paths"
	blockPathFormat   = `Adding label:%s because PR changes docs prohibited to auto merge
See http://kubernetes.io/editdocs/ for information about editing docs`
)

var (
	_                       = fmt.Print
	blockPathBody           = fmt.Sprintf(blockPathFormat, blockedPathsLabel)
	deprecatedBlockPathBody = fmt.Sprintf(blockPathFormat, doNotMergeLabel)
)

type configBlockPath struct {
	BlockRegexp      []string `json:"blockRegexp,omitempty" yaml:"blockRegexp,omitempty"`
	DoNotBlockRegexp []string `json:"doNotBlockRegexp,omitempty" yaml:"doNotBlockRegexp,omitempty"`
}

// BlockPath will add a label to block auto merge if a PR touches certain paths
type BlockPath struct {
	path             string
	blockRegexp      []*regexp.Regexp
	doNotBlockRegexp []*regexp.Regexp
}

func init() {
	b := &BlockPath{}
	RegisterMungerOrDie(b)
	RegisterStaleIssueComments(b)
}

// Name is the name usable in --pr-mungers
func (b *BlockPath) Name() string { return "block-path" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *BlockPath) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (b *BlockPath) Initialize(config *github.Config, features *features.Features) error {
	return b.loadConfig()
}

func (b *BlockPath) loadConfig() error {
	if len(b.path) == 0 {
		return fmt.Errorf("'block-path-config' option is required with the block-path munger")
	}
	file, err := os.Open(b.path)
	if err != nil {
		return fmt.Errorf("Failed to load block-path config: %v", err)
	}
	defer file.Close()

	c := &configBlockPath{}
	if err := yaml.NewYAMLToJSONDecoder(file).Decode(c); err != nil {
		return fmt.Errorf("Failed to decode the block-path config: %v", err)
	}

	blockRegexp := []*regexp.Regexp{}
	for _, str := range c.BlockRegexp {
		reg, err := regexp.Compile(str)
		if err != nil {
			return err
		}
		blockRegexp = append(blockRegexp, reg)
	}

	doNotBlockRegexp := []*regexp.Regexp{}
	for _, str := range c.DoNotBlockRegexp {
		reg, err := regexp.Compile(str)
		if err != nil {
			return err
		}
		doNotBlockRegexp = append(doNotBlockRegexp, reg)
	}

	b.blockRegexp = blockRegexp
	b.doNotBlockRegexp = doNotBlockRegexp
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *BlockPath) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (b *BlockPath) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&b.path, "block-path-config", "", "file containing the pathnames to block or not block")
	opts.RegisterUpdateCallback(func(changed sets.String) error {
		if changed.Has("block-path-config") {
			if err := b.loadConfig(); err != nil {
				glog.Errorf("error reloading block-path-config: %v", err)
			}
		}
		return nil
	})
	return nil
}

func matchesAny(path string, regs []*regexp.Regexp) bool {
	for _, reg := range regs {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

// Munge is the workhorse the will actually make updates to the PR
func (b *BlockPath) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if obj.HasLabel(blockedPathsLabel) {
		return
	}

	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	for _, f := range files {
		if matchesAny(*f.Filename, b.blockRegexp) {
			if matchesAny(*f.Filename, b.doNotBlockRegexp) {
				continue
			}
			obj.WriteComment(blockPathBody)
			obj.AddLabels([]string{blockedPathsLabel})
			return
		}
	}
}

func (b *BlockPath) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if *comment.Body != blockPathBody && *comment.Body != deprecatedBlockPathBody {
		return false
	}
	stale := !obj.HasLabel(blockedPathsLabel)
	if stale {
		glog.V(6).Infof("Found stale BlockPath comment")
	}
	return stale
}

// StaleIssueComments returns a slice of stale issue comments.
func (b *BlockPath) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, b.isStaleIssueComment)
}
