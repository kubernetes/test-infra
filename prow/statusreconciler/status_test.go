/*
Copyright 2018 The Kubernetes Authors.

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

package statusreconciler

import (
	"context"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/io"
)

type testOpener struct{}

func (t *testOpener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (t *testOpener) Writer(ctx context.Context, path string, _ ...io.WriterOptions) (io.WriteCloser, error) {
	return os.Create(path)
}

func TestLoadState(t *testing.T) {
	config := config.Config{
		ProwConfig: config.ProwConfig{
			ProwJobNamespace: "default",
		},
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"org/repo": getPresubmits([]string{"foo"}),
			},
		},
	}
	configFile, cleanup := getConfigFile(t, config)
	defer cleanup()

	sc := statusController{
		logger:    logrus.NewEntry(logrus.StandardLogger()),
		statusURI: configFile,
		opener:    &testOpener{},
	}

	got, err := sc.loadState()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.Config, config) {
		t.Errorf("Expected result %#v', got %#v", config, got)
	}
}

func TestLoad(t *testing.T) {
	presubmitFoo := "presubmit-foo"
	presubmitBar := "presubmit-bar"

	savedConfig := config.Config{
		ProwConfig: config.ProwConfig{
			ProwJobNamespace: "default",
		},
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"org/repo": getPresubmits([]string{presubmitFoo}),
			},
		},
	}

	newProwConfig := config.ProwConfig{
		ProwJobNamespace: "foo",
	}

	newJobConfig := config.JobConfig{
		PresubmitsStatic: map[string][]config.Presubmit{
			"org/repo": getPresubmits([]string{presubmitFoo, presubmitBar}),
		},
	}

	testCases := []struct {
		name               string
		existingStatusFile bool
		savedNamespace     string
		savedPresubmits    []string
		newNamespace       string
		newPresubmits      []string
	}{
		{
			name:          "no status file should not cause any errors",
			newNamespace:  "foo",
			newPresubmits: []string{"presubmit-bar", "presubmit-foo"},
		},
		{
			name:               "With an existing status file, configuration changes since last saved should be identified",
			existingStatusFile: true,
			savedNamespace:     "default",
			savedPresubmits:    []string{"presubmit-foo"},
			newNamespace:       "foo",
			newPresubmits:      []string{"presubmit-bar", "presubmit-foo"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusURI := ""
			if tc.existingStatusFile {
				statusFile, cleanupStatusFile := getConfigFile(t, savedConfig)
				defer cleanupStatusFile()
				statusURI = statusFile
			}

			configFile, cleanupConfig := getConfigFile(t, config.Config{ProwConfig: newProwConfig})
			defer cleanupConfig()

			jobConfigFile, cleanupJobConfig := getConfigFile(t, config.Config{JobConfig: newJobConfig})
			defer cleanupJobConfig()

			sc := statusController{
				logger:    logrus.NewEntry(logrus.StandardLogger()),
				statusURI: statusURI,
				opener:    &testOpener{},
				configOpts: configflagutil.ConfigOptions{
					ConfigPath:    configFile,
					JobConfigPath: jobConfigFile,
				},
			}
			changes, err := sc.Load()
			if err != nil {
				t.Fatalf("%s: unexpected error %v", tc.name, err)
			}
			select {
			case change := <-changes:
				verify(t, tc.name+"/before", change.Before, tc.savedNamespace, tc.savedPresubmits)
				verify(t, tc.name+"/after", change.After, tc.newNamespace, tc.newPresubmits)
			case <-time.After(3 * time.Second):
				t.Fatalf("%s: unexpected timeout while waiting for configuration changes", tc.name)
			}
		})
	}
}

func TestSave(t *testing.T) {
	presubmitFoo := "presubmit-foo"
	presubmitBar := "presubmit-bar"
	prowConfig := config.ProwConfig{
		ProwJobNamespace: "default",
	}
	jobConfig := config.JobConfig{
		PresubmitsStatic: map[string][]config.Presubmit{
			"org/repo": getPresubmits([]string{presubmitFoo, presubmitBar}),
		},
	}

	configFile, cleanupConfig := getConfigFile(t, config.Config{ProwConfig: prowConfig})
	defer cleanupConfig()

	jobConfigFile, cleanupJobConfig := getConfigFile(t, config.Config{JobConfig: jobConfig})
	defer cleanupJobConfig()

	testCases := []struct {
		name               string
		specifyStatusFile  bool
		expectedNamespace  string
		expectedPresubmits []string
	}{
		{
			name: "not specifying a status file should not cause any errors",
		},
		{
			name:               "save stores the configuration in the specified statusURI",
			expectedNamespace:  "default",
			expectedPresubmits: []string{"presubmit-bar", "presubmit-foo"},
		},
	}

	for _, tc := range testCases {
		statusURI := ""
		if tc.specifyStatusFile {
			statusFile, cleanupStatusFile := getConfigFile(t, config.Config{})
			defer cleanupStatusFile()
			statusURI = statusFile
		}

		t.Run(tc.name, func(t *testing.T) {
			sc := statusController{
				logger:    logrus.NewEntry(logrus.StandardLogger()),
				statusURI: statusURI,
				opener:    &testOpener{},
				configOpts: configflagutil.ConfigOptions{
					ConfigPath:    configFile,
					JobConfigPath: jobConfigFile,
				},
			}
			if err := sc.Save(); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}

			if statusURI != "" {
				buf, err := ioutil.ReadFile(statusURI)
				if err != nil {
					t.Fatalf("%s: unexpected error reading status file: %v", tc.name, err)
				}

				var got config.Config
				if err := yaml.Unmarshal(buf, &got); err != nil {
					t.Fatalf("%s: unexpected error unmarshaling status file contents %v", tc.name, err)
				}
				verify(t, tc.name, got, tc.expectedNamespace, tc.expectedPresubmits)
			}
		})
	}
}

func getConfigFile(t *testing.T, config config.Config) (string, func()) {
	tempFile, err := ioutil.TempFile("/tmp", "prow-test")
	if err != nil {
		t.Fatalf("failed to get tempfile: %v", err)
	}
	cleanup := func() {
		if err := tempFile.Close(); err != nil {
			t.Errorf("failed to close tempFile: %v", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("failed to remove tempfile: %v", err)
		}
	}

	buf, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Cannot marshal config: %v", err)
	}

	if _, err := tempFile.Write(buf); err != nil {
		t.Fatalf("failed to write to tempfile: %v", err)
	}

	return tempFile.Name(), cleanup
}

func getPresubmits(names []string) []config.Presubmit {
	spec := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "image",
			},
		},
	}

	var presubmits []config.Presubmit
	for _, name := range names {
		ps := config.Presubmit{
			JobBase: config.JobBase{
				Name: name,
				Spec: spec,
			},
			AlwaysRun: true,
			Reporter:  config.Reporter{Context: name},
		}
		presubmits = append(presubmits, ps)
	}
	return presubmits
}

func verify(t *testing.T, testCase string, config config.Config, Namespace string, presubmits []string) {
	if config.ProwConfig.ProwJobNamespace != Namespace {
		t.Errorf("%s: expected namespace %s, got %s", testCase, Namespace, config.ProwConfig.ProwJobNamespace)
	}
	var names []string
	for _, ps := range config.JobConfig.PresubmitsStatic["org/repo"] {
		names = append(names, ps.JobBase.Name)
	}
	sort.Strings(names)
	sort.Strings(presubmits)
	if !reflect.DeepEqual(names, presubmits) {
		t.Errorf("%s: expected presubmit names %v, got %v", testCase, presubmits, names)
	}
}
