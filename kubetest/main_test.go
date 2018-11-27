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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"k8s.io/test-infra/kubetest/util"
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
			key:     "job-version",
			version: "v1.8.0-alpha.2.251+ba2bdb1aead615",
		},
		{
			sources: map[string]mdata{"images.json": {"imgkey": "imgval"}},
			key:     "imgkey",
			version: "v1.8.0-alpha.2.251+ba2bdb1aead615",
		},
		{
			sources: map[string]mdata{
				"images.json": {"imgkey": "imgval"},
				"foo.json":    {"fookey": "fooval"},
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

func TestMigrateGcpEnvAndOptions(t *testing.T) {
	proj := "fake-project"
	zone := "fake-zone"
	cases := []struct {
		name        string
		provider    string
		expectedArg string
	}{
		{
			name:        "gce sets KUBE_GCE_ZONE",
			provider:    "gce",
			expectedArg: "KUBE_GCE_ZONE",
		},
		{
			name:        "gke sets ZONE",
			provider:    "gke",
			expectedArg: "ZONE",
		},
		{
			name:        "random provider sets KUBE_GCE_ZONE",
			provider:    "random",
			expectedArg: "KUBE_GCE_ZONE",
		},
	}

	// Preserve original ZONE, KUBE_GCE_ZONE state
	if pz, err := util.PushEnv("ZONE", "unset"); err != nil {
		t.Fatalf("Could not set ZONE: %v", err)
	} else {
		defer pz()
	}
	if pkgz, err := util.PushEnv("KUBE_GCE_ZONE", "unset"); err != nil {
		t.Fatalf("Could not set KUBE_GCE_ZONE: %v", err)
	} else {
		defer pkgz()
	}

	for _, tc := range cases {
		if err := os.Unsetenv("KUBE_GCE_ZONE"); err != nil {
			t.Fatalf("%s: could not unset KUBE_GCE_ZONE", tc.name)
		}
		if err := os.Unsetenv("ZONE"); err != nil {
			t.Fatalf("%s: could not unset ZONE", tc.name)
		}
		o := options{
			gcpProject: proj,
			gcpZone:    zone,
			provider:   tc.provider,
		}
		if err := migrateGcpEnvAndOptions(&o); err != nil {
			t.Errorf("%s: failed to migrate: %v", tc.name, err)
		}

		z := os.Getenv(tc.expectedArg)
		if z != zone {
			t.Errorf("%s: %s is '%s' not expected '%s'", tc.name, tc.expectedArg, z, zone)
		}
	}
}

func TestPrepareParallelism(t *testing.T) {
	cases := []struct {
		initial             []string
		ginkgoParallel      string
		ginkgoParallelNodes string
		wantParallel        int
	}{
		{
			wantParallel: 1,
		},
		{
			initial:      []string{"10"},
			wantParallel: 10,
		},
		{
			initial:      []string{"true"},
			wantParallel: defaultGinkgoParallel,
		},
		{
			initial:      []string{"true", "20"},
			wantParallel: 20,
		},
		{
			ginkgoParallel: "y",
			wantParallel:   defaultGinkgoParallel,
		},
		{
			ginkgoParallel:      "y",
			ginkgoParallelNodes: "50",
			wantParallel:        50,
		},
		{
			ginkgoParallelNodes: "50",
			wantParallel:        50,
		},
		{
			initial:             []string{"20"},
			ginkgoParallelNodes: "50",
			wantParallel:        50,
		},
	}

	// Preserve original GINKGO_PARALLEL and GINKGO_PARALLEL_NODES
	if pre, err := util.PushEnv("GINKGO_PARALLEL", "unset"); err != nil {
		t.Fatalf("Could not set GINKGO_PARALLEL: %v", err)
	} else {
		defer pre()
	}
	if pre, err := util.PushEnv("GINKGO_PARALLEL_NODES", "unset"); err != nil {
		t.Fatalf("Could not set GINKGO_PARALLEL_NODES: %v", err)
	} else {
		defer pre()
	}

	for _, tc := range cases {
		desc := fmt.Sprintf("(%v, %q, %q)", tc.initial, tc.ginkgoParallel, tc.ginkgoParallelNodes)
		if err := os.Setenv("GINKGO_PARALLEL", tc.ginkgoParallel); err != nil {
			t.Fatalf("%s => could not unset GINKGO_PARALLEL", desc)
		}
		if err := os.Setenv("GINKGO_PARALLEL_NODES", tc.ginkgoParallelNodes); err != nil {
			t.Fatalf("%s => could not unset GINKGO_PARALLEL_NODES", desc)
		}
		v := ginkgoParallelValue{}
		for _, i := range tc.initial {
			if err := v.Set(i); err != nil {
				t.Fatalf("%s => could not .Set(%q): %v", desc, i, err)
			}
		}
		if err := prepareGinkgoParallel(&v); err != nil {
			t.Errorf("%s => error %v, did not want", desc, err)
		}

		if i := v.Get(); i != tc.wantParallel {
			t.Errorf("%s => parallel %d (got) != %d (want)", desc, i, tc.wantParallel)
		}
		if gp := os.Getenv("GINKGO_PARALLEL"); gp != "" {
			t.Errorf("%s => GINKGO_PARALLEL is set to %q, did not want", desc, gp)
		}
		if gpn := os.Getenv("GINKGO_PARALLEL_NODES"); gpn != strconv.Itoa(v.Get()) {
			t.Errorf("%s => GINKGO_PARALLEL_NODES=%s (got) != %s (want)", desc, gpn, v.String())
		}
	}
}
