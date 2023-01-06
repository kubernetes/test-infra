/*
Copyright 2019 The Kubernetes Authors.

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

package tests

import (
	"os"
	"path/filepath"
	"testing"
)

var configPath = "../../../config/"

var exemptPaths = []string{
	"prow/cluster/monitoring/mixins/vendor",
}

func Test_ForbidYmlExtension(t *testing.T) {
	exempt := map[string]bool{}
	for _, path := range exemptPaths {
		exempt[filepath.Join(configPath, path)] = true
	}
	err := filepath.Walk(configPath, func(path string, info os.FileInfo, err error) error {
		if _, ok := exempt[path]; ok {
			return filepath.SkipDir
		}
		if filepath.Ext(path) == ".yml" {
			t.Errorf("*.yml extension not allowed in this repository's configuration; use *.yaml instead (at %s)", path)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}
