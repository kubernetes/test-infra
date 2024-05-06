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

package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/pkg/genyaml"
	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/prow/pkg/plugins"
)

const (
	// Pretending that it runs from root of the repo
	defaultRootDir = "."
)

var genConfigs = []genConfig{
	{
		in: []string{
			"prow/config/*.go",
			"prow/apis/prowjobs/v1/*.go",
		},
		format: &config.ProwConfig{},
		out:    "prow/config/prow-config-documented.yaml",
	},
	{
		in: []string{
			"prow/plugins/*.go",
		},
		format: &plugins.Configuration{},
		out:    "prow/plugins/plugin-config-documented.yaml",
	},
}

type genConfig struct {
	in     []string
	format interface{}
	out    string
}

func (g *genConfig) gen(rootDir string) error {
	var inputFiles []string
	for _, goGlob := range g.in {
		ifs, err := filepath.Glob(path.Join(rootDir, goGlob))
		if err != nil {
			return fmt.Errorf("filepath glob: %w", err)
		}
		inputFiles = append(inputFiles, ifs...)
	}

	commentMap, err := genyaml.NewCommentMap(nil, inputFiles...)
	if err != nil {
		return fmt.Errorf("failed to construct commentMap: %w", err)
	}
	actualYaml, err := commentMap.GenYaml(genyaml.PopulateStruct(g.format))
	if err != nil {
		return fmt.Errorf("genyaml errored: %w", err)
	}
	if err := os.WriteFile(path.Join(rootDir, g.out), []byte(actualYaml), 0644); err != nil {
		return fmt.Errorf("failed to write fixture: %w", err)
	}
	return nil
}

func main() {
	rootDir := flag.String("root-dir", defaultRootDir, "Repo root dir.")
	flag.Parse()

	for _, g := range genConfigs {
		if err := g.gen(*rootDir); err != nil {
			logrus.WithError(err).WithField("fixture", g.out).Error("Failed generating.")
			os.Exit(1)
		}
	}
}
