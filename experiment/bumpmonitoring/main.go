/*
Copyright 2021 The Kubernetes Authors.

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

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/cmd/generic-autobumper/bumper"
	"sigs.k8s.io/yaml"
)

const (
	defaultSrcPath = "../../config/prow/cluster/monitoring"
)

var (
	configPathsToUpdate = map[string]*regexp.Regexp{
		"mixins/grafana_dashboards": regexp.MustCompile(`.*\.jsonnet$`),
		"mixins/prometheus":         regexp.MustCompile(`.*\.libsonnet$`),
	}
	configPathExcluded = []*regexp.Regexp{
		regexp.MustCompile(`mixins/prometheus/prometheus\.libsonnet`),
	}
)

type options struct {
	SrcPath string `json:"srcPath"`
	DstPath string `json:"dstPath"`
	bumper.Options
}

func parseOptions() (*options, error) {
	var config string
	var labelsOverride []string
	var skipPullRequest bool

	flag.StringVar(&config, "config", "", "The path to the config file for PR creation.")
	flag.StringSliceVar(&labelsOverride, "labels-override", nil, "Override labels to be added to PR.")
	flag.BoolVar(&skipPullRequest, "skip-pullrequest", false, "")
	flag.Parse()

	var o options
	data, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", config, err)
	}
	if err = yaml.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("unmarshal %q: %w", config, err)
	}

	if len(o.SrcPath) == 0 {
		o.SrcPath = defaultSrcPath
	}
	if labelsOverride != nil {
		o.Labels = labelsOverride
	}
	o.SkipPullRequest = skipPullRequest
	return &o, nil
}

func validateOptions(o *options) error {
	if len(o.DstPath) == 0 {
		return errors.New("dstPath is mandatory")
	}

	return nil
}

var _ bumper.PRHandler = (*client)(nil)

type client struct {
	srcPath string
	dstPath string
	paths   []string
}

// Changes returns a slice of functions, each one does some stuff, and
// returns commit message for the changes
func (c *client) Changes() []func() (string, error) {
	return []func() (string, error){
		func() (string, error) {
			if err := c.findConfigToUpdate(); err != nil {
				return "", err
			}

			if err := c.copyFiles(); err != nil {
				return "", err
			}

			return strings.Join([]string{c.title(), c.body()}, "\n\n"), nil
		},
	}
}

// PRTitleBody returns the body of the PR, this function runs after each commit
func (c *client) PRTitleBody() (string, string, error) {
	return c.title(), c.body(), nil
}

func (c *client) title() string {
	return "Update monitoring stack"
}

func (c *client) body() string {
	return fmt.Sprintf(`For code reviewers:
- breaking changes are only introduced in %s/mixins/lib/config.libsonnet
- presubmit test is expected to fail if there is any breaking change
- push to this change with fix if it's the case`, c.dstPath)
}

func (c *client) findConfigToUpdate() error {
	for subPath, re := range configPathsToUpdate {
		fullPath := path.Join(c.dstPath, subPath)

		if _, err := os.Stat(fullPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to get the file info for %q: %v", fullPath, err)
			}
			logrus.Infof("Skipping %s as it doesn't exist from dst", fullPath)
		}

		// No error is expected
		filepath.Walk(fullPath, func(leafPath string, info os.FileInfo, err error) error {
			if !re.MatchString(leafPath) {
				return nil
			}
			for _, reExcluded := range configPathExcluded {
				if reExcluded.MatchString(leafPath) {
					return nil
				}
			}
			relPath, _ := filepath.Rel(c.dstPath, leafPath)
			c.paths = append(c.paths, relPath)
			return nil
		})
	}

	return nil
}

func (c *client) copyFiles() error {
	for _, subPath := range c.paths {
		SrcPath := path.Join(c.srcPath, subPath)
		DstPath := path.Join(c.dstPath, subPath)
		content, err := ioutil.ReadFile(SrcPath)
		if err != nil {
			return fmt.Errorf("failed reading file %q: %w", SrcPath, err)
		}
		if err := ioutil.WriteFile(DstPath, content, 0755); err != nil {
			return fmt.Errorf("failed writing file %q: %w", DstPath, err)
		}
	}
	return nil
}

func main() {
	o, err := parseOptions()
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to run the bumper tool")
	}
	if err := validateOptions(o); err != nil {
		logrus.WithError(err).Fatalf("Failed validating flags")
	}

	c := client{
		srcPath: o.SrcPath,
		dstPath: o.DstPath,
		paths:   make([]string, 0),
	}

	if err := bumper.Run(&o.Options, &c); err != nil {
		logrus.Fatal(err)
	}
}
