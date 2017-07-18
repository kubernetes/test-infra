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

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMetadata(t *testing.T) {
	type mdata map[string]string
	cases := []struct {
		sources map[string]mdata
		key     string
		version string
	}{
		{
			sources: nil,
			key:     "version",
			version: "v1.8.0-alpha.2.251+ba2bdb1aead615",
		},
		{
			sources: map[string]mdata{"images.json": mdata{"imgkey": "imgval"}},
			key:     "imgkey",
			version: "v1.8.0-alpha.2.251+ba2bdb1aead615",
		},
		{
			sources: map[string]mdata{
				"images.json": mdata{"imgkey": "imgval"},
				"foo.json":    mdata{"fookey": "fooval"},
			},
			key:     "fookey",
			version: "v1.8.0-alpha.2.251+ba2bdb1aead615",
		},
	}

	writeTmpMetadataSource := func(filePath string, md mdata) {
		outputBytes, _ := json.MarshalIndent(md, "", "  ")
		if err := ioutil.WriteFile(filePath, outputBytes, 0644); err != nil {
			t.Fatalf("write to %q: %v", filePath, err)
		}
	}

	for _, tc := range cases {
		topDir, err := ioutil.TempDir("", "TestWriteMetadata")

		if err != nil {
			t.Fatal(err)
		}

		defer os.RemoveAll(topDir) // Stack up all the cleanups

		dumpDir := filepath.Join(topDir, "artifacts")
		if err := os.Mkdir(dumpDir, 0755); err != nil {
			t.Fatal(err)
		}

		if err := ioutil.WriteFile(filepath.Join(topDir, "version"), []byte(tc.version+"\n"), 0644); err != nil {
			t.Fatalf("write %q/version: %v", topDir, err)
		}
		sourceNames := []string{}
		for filename, metadata := range tc.sources {
			sourceNames = append(sourceNames, filename)
			writeTmpMetadataSource(filepath.Join(dumpDir, filename), metadata)
		}

		// Now we've set things up, call the function
		//
		os.Chdir(topDir) // version file is read from "."
		writeMetadata(dumpDir, strings.Join(sourceNames, ","))

		// Load up the output
		metadata := map[string]string{}
		maybeMergeJSON(metadata, filepath.Join(dumpDir, "metadata.json"))

		if _, exists := metadata[tc.key]; !exists {
			t.Errorf("Expcected metadata key %q, but read in map %#v\n", tc.key, metadata)
		}
	}
}
