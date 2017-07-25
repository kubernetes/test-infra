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

package features

import (
	"fmt"
	"io/ioutil"
	"os"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
)

const (
	// AliasesFeature is how mungers should indicate this is required.
	AliasesFeature = "aliases"
)

type aliasData struct {
	// Contains the mapping between aliases and lists of members.
	AliasMap map[string][]string `json:"aliases"`
}

type aliasReader interface {
	read() ([]byte, error)
}

func (a *Aliases) read() ([]byte, error) {
	return ioutil.ReadFile(a.aliasFile)
}

// Aliases is a struct that handles parameters required by mungers
// to expand and lookup aliases.
type Aliases struct {
	aliasFile string

	data        *aliasData
	prevHash    string
	aliasReader aliasReader
}

var _ feature = &Aliases{}

func init() {
	RegisterFeature(&Aliases{})
}

// Name is just going to return the name mungers use to request this feature
func (a *Aliases) Name() string {
	return AliasesFeature
}

// Initialize will initialize the feature.
func (a *Aliases) Initialize(config *github.Config) error {
	a.data = &aliasData{}
	a.aliasReader = a

	return nil
}

// EachLoop is called at the start of every munge loop
func (a *Aliases) EachLoop() error {
	if len(a.aliasFile) == 0 {
		return nil
	}

	// read and check the alias-file.
	fileContents, err := a.aliasReader.read()
	if os.IsNotExist(err) {
		glog.Infof("Missing alias-file (%s), using empty alias structure.", a.aliasFile)
		a.data = &aliasData{}
		a.prevHash = ""
		return nil
	}
	if err != nil {
		return fmt.Errorf("Unable to read alias file: %v", err)
	}

	hash := mungerutil.GetHash(fileContents)
	if a.prevHash != hash {
		var data aliasData
		if err := yaml.Unmarshal(fileContents, &data); err != nil {
			return fmt.Errorf("Failed to decode the alias file: %v", err)
		}
		a.data = &data
		a.prevHash = hash
	}
	return nil
}

// RegisterOptions registers options for this feature; returns any that require a restart when changed.
func (a *Aliases) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&a.aliasFile, "alias-file", "", "File wherein team members and aliases exist.")
	return nil
}

// Expand takes aliases and expands them into owner lists.
func (a *Aliases) Expand(toExpand sets.String) sets.String {
	expanded := sets.String{}
	for _, owner := range toExpand.List() {
		expanded.Insert(a.resolve(owner)...)
	}
	return expanded
}

func (a *Aliases) resolve(owner string) []string {
	if val, ok := a.data.AliasMap[owner]; ok {
		return val
	}
	return []string{owner}
}
