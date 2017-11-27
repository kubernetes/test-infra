/*
Copyright 2015 The Kubernetes Authors.

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
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
)

var (
	_ = fmt.Print
)

const (
	jenkinsBotName = "k8s-bot"
)

type labelMap struct {
	regexp *regexp.Regexp
	label  string
}

// PathLabelMunger will add labels to PRs based on what files it modified.
// The mapping of files to labels if provided in a file in --path-label-config
type PathLabelMunger struct {
	pathLabelFile string
	labelMap      []labelMap
	allLabels     sets.String
}

func init() {
	RegisterMungerOrDie(&PathLabelMunger{})
}

// Name is the name usable in --pr-mungers
func (p *PathLabelMunger) Name() string { return "path-label" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *PathLabelMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (p *PathLabelMunger) Initialize(config *github.Config, features *features.Features) error {
	allLabels := sets.NewString()
	out := []labelMap{}
	file := p.pathLabelFile
	if len(file) == 0 {
		glog.Infof("No 'path-label-config' option supplied, applying no labels.")
		return nil
	}
	fp, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fp.Close()
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			glog.Errorf("Invalid line in path based label munger config %s: %q", file, line)
			continue
		}
		r, err := regexp.Compile(fields[0])
		if err != nil {
			glog.Errorf("Invalid regexp in label munger config %s: %q", file, fields[0])
			continue
		}

		label := fields[1]
		lm := labelMap{
			regexp: r,
			label:  label,
		}
		out = append(out, lm)
		allLabels.Insert(label)
	}
	p.allLabels = allLabels
	p.labelMap = out
	return scanner.Err()
}

// EachLoop is called at the start of every munge loop
func (p *PathLabelMunger) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (p *PathLabelMunger) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&p.pathLabelFile, "path-label-config", "", "file containing the pathname to label mappings")
	opts.RegisterUpdateCallback(func(changed sets.String) error {
		if changed.Has("path-label-config") {
			return p.Initialize(nil, nil) // Initialize doesn't use config or features.
		}
		return nil
	})
	return nil
}

// Munge is the workhorse the will actually make updates to the PR
func (p *PathLabelMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	needsLabels := sets.NewString()
	for _, f := range files {
		for _, lm := range p.labelMap {
			if lm.regexp.MatchString(*f.Filename) {
				needsLabels.Insert(lm.label)
			}
		}
	}

	SyncLabels(p.allLabels, needsLabels, obj)
}

// SyncLabels properly syncs a set of labels. 'allLabels' must be a superset of
// 'desiredLabels'; to disable removing labels, set them to be the same set.
// Multiple mungers must somehow coordinate on which labels the bot ought to
// apply, otherwise the bot will fight with itself.
//
// TODO: fix error handling.
func SyncLabels(allLabels, desiredLabels sets.String, obj *github.MungeObject) error {
	hasLabels := obj.LabelSet().Intersection(allLabels)

	missingLabels := desiredLabels.Difference(hasLabels)
	if missingLabels.Len() != 0 {
		obj.AddLabels(missingLabels.List())
	}

	extraLabels := hasLabels.Difference(desiredLabels)
	for _, label := range extraLabels.List() {
		creator, ok := obj.LabelCreator(label)
		if ok && obj.IsRobot(creator) {
			obj.RemoveLabel(label)
		}
	}
	return nil
}
