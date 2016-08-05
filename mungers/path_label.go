/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Print
)

const (
	botName        = "k8s-merge-robot"
	jenkinsBotName = "k8s-bot"
)

type labelMap struct {
	regexp *regexp.Regexp
	label  string
}

// PathLabelMunger will add labels to PRs based on what files it modified.
// The mapping of files to labels if provided in a file in --path-label-config
type PathLabelMunger struct {
	PathLabelFile string
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
	file := p.PathLabelFile
	if len(file) == 0 {
		glog.Infof("No --path-label-config= supplied, applying no labels")
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

// AddFlags will add any request flags to the cobra `cmd`
func (p *PathLabelMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&p.PathLabelFile, "path-label-config", "", "file containing the pathname to label mappings")
}

// Munge is the workhorse the will actually make updates to the PR
func (p *PathLabelMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	files, err := obj.ListFiles()
	if err != nil {
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

	// This is all labels on the issue that the path munger controls
	hasLabels := obj.LabelSet().Intersection(p.allLabels)

	missingLabels := needsLabels.Difference(hasLabels)
	if missingLabels.Len() != 0 {
		obj.AddLabels(needsLabels.List())
	}

	extraLabels := hasLabels.Difference(needsLabels)
	for _, label := range extraLabels.List() {
		creator := obj.LabelCreator(label)
		if creator == botName {
			obj.RemoveLabel(label)
		}
	}
}
