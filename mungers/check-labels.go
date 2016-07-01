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
	"fmt"
	"os"

	"k8s.io/contrib/mungegithub/features"
	githubhelper "k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/yaml"

	"bytes"
	"crypto/sha1"
	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"io/ioutil"
)

type labelAccessor interface {
	AddLabel(label *github.Label) error
	GetLabels() ([]*github.Label, error)
}

// CheckLabelsMunger will check that the labels specified in the labels yaml file
// are created.
type CheckLabelsMunger struct {
	labelFilePath string
	prevHash      string
	labelAccessor labelAccessor
	features      *features.Features
	readFunc      func() ([]byte, error)
}

func init() {
	RegisterMungerOrDie(&CheckLabelsMunger{})
}

// Name is the name usable in --pr-mungers
func (c *CheckLabelsMunger) Name() string { return "check-labels" }

// RequiredFeatures is a slice of 'features' that must be provided.
func (c *CheckLabelsMunger) RequiredFeatures() []string { return []string{features.RepoFeatureName} }

// Initialize will initialize the munger.
func (c *CheckLabelsMunger) Initialize(config *githubhelper.Config, features *features.Features) error {
	if len(c.labelFilePath) == 0 {
		glog.Fatalf("No --label-file= supplied, cannot check labels")
	}
	c.labelAccessor = config
	c.features = features
	c.readFunc = func() ([]byte, error) {
		bytes, err := ioutil.ReadFile(c.labelFilePath)
		if err != nil {
			return []byte{}, fmt.Errorf("Unable to read label file: %v", err)
		}
		return bytes, nil
	}

	if _, err := os.Stat(c.labelFilePath); os.IsNotExist(err) {
		return fmt.Errorf("Failed to stat the check label config: %v", err)
	}

	return nil
}

func (c *CheckLabelsMunger) getHash(fileContents []byte) string {
	h := sha1.New()
	h.Write([]byte(fileContents))
	bs := h.Sum(nil)
	return string(bs)
}

// EachLoop is called at the start of every munge loop
func (c *CheckLabelsMunger) EachLoop() error {
	fileContents, err := c.readFunc()
	if err != nil {
		glog.Errorf("Failed to read the check label config: %v", err)
		return err
	}
	hash := c.getHash(fileContents)
	if c.prevHash != hash {
		// Get all labels from file.
		fileLabels := map[string][]*github.Label{}
		if err := yaml.NewYAMLToJSONDecoder(bytes.NewReader(fileContents)).Decode(&fileLabels); err != nil {
			return fmt.Errorf("Failed to decode the check label config: %v", err)
		}

		// Get all labels from repository.
		repoLabels, err := c.labelAccessor.GetLabels()
		if err != nil {
			return err
		}
		c.addMissingLabels(repoLabels, fileLabels["labels"])
		c.prevHash = hash
	}
	return nil
}

// addMissingLabels will not remove any labels. It will add those which are present in the yaml file and not in
// the repository.
func (c *CheckLabelsMunger) addMissingLabels(repoLabels, fileLabels []*github.Label) {
	repoLabelSet := sets.NewString()
	for _, repoLabel := range repoLabels {
		repoLabelSet.Insert(*repoLabel.Name)
	}

	// Compare against labels in local file.
	for _, label := range fileLabels {
		if !repoLabelSet.Has(*label.Name) {
			err := c.labelAccessor.AddLabel(label)
			if err != nil {
				glog.Errorf("Error %s in adding label %s", err, *label.Name)
			}
		}
	}
}

// AddFlags will add any request flags to the cobra `cmd`.
func (c *CheckLabelsMunger) AddFlags(cmd *cobra.Command, config *githubhelper.Config) {
	cmd.Flags().StringVar(&c.labelFilePath, "label-file", "", "Path from repository root to file containing"+
		" list of labels")
}

// Munge is unused by this munger.
func (c *CheckLabelsMunger) Munge(obj *githubhelper.MungeObject) {}
