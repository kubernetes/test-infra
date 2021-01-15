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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/sirupsen/logrus"
)

const (
	srcPath = "../../config/prow/cluster/monitoring"
)

var (
	configPathsToUpdate = map[string]*regexp.Regexp{
		"mixins/grafana_dashboards": regexp.MustCompile(`.*\.jsonnet$`),
		"mixins/prometheus":         regexp.MustCompile(`.*\.libsonnet$`),
	}
	configPathExcluded = []*regexp.Regexp{
		regexp.MustCompile(`mixins/prometheus/prometheus\.libsonnet`),
	}
	configPathProwSpecific = []string{
		"mixins/lib/config_util.libsonnet",
	}
)

type options struct {
	srcPath string
	dstPath string
}

type client struct {
	srcPath string
	dstPath string
	paths   []string
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
		srcPath := path.Join(c.srcPath, subPath)
		dstPath := path.Join(c.dstPath, subPath)
		content, err := ioutil.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed reading file %q: %w", srcPath, err)
		}
		if err := ioutil.WriteFile(dstPath, content, 0755); err != nil {
			return fmt.Errorf("failed writing file %q: %w", dstPath, err)
		}
	}
	return nil
}

func (c *client) generateMsg() string {
	return fmt.Sprintf(`Update monitoring stack

For code reviewers:
- breaking changes are only introduced in %s/mixins/lib/config.libsonnet
- presubmit test is expected to fail if there is any breaking change
- push to this change with fix if it's the case`, c.dstPath)
}

func main() {
	o := options{}
	flag.StringVar(&o.srcPath, "src", "", "Src dir of monitoring")
	flag.StringVar(&o.dstPath, "dst", "", "Dst dir of monitoring")
	flag.Parse()

	c := client{
		srcPath: o.srcPath,
		dstPath: o.dstPath,
		paths:   make([]string, 0),
	}

	if err := c.findConfigToUpdate(); err != nil {
		logrus.Fatal(err)
	}

	if err := c.copyFiles(); err != nil {
		logrus.Fatal(err)
	}
}
