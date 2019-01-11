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

package entrypoint

import (
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

func TestOptions_Run(t *testing.T) {
	var testCases = []struct {
		name           string
		args           []string
		timeout        time.Duration
		gracePeriod    time.Duration
		expectedLog    string
		expectedMarker string
	}{
		{
			name:           "successful command",
			args:           []string{"sh", "-c", "exit 0"},
			expectedLog:    "",
			expectedMarker: "0",
		},
		{
			name:           "successful command with output",
			args:           []string{"echo", "test"},
			expectedLog:    "test\n",
			expectedMarker: "0",
		},
		{
			name:           "unsuccessful command",
			args:           []string{"sh", "-c", "exit 12"},
			expectedLog:    "",
			expectedMarker: "12",
		},
		{
			name:           "unsuccessful command with output",
			args:           []string{"sh", "-c", "echo test && exit 12"},
			expectedLog:    "test\n",
			expectedMarker: "12",
		},
		{
			name:           "command times out",
			args:           []string{"sleep", "10"},
			timeout:        1 * time.Second,
			gracePeriod:    1 * time.Second,
			expectedLog:    "level=error msg=\"Process did not finish before 1s timeout\" \nlevel=error msg=\"Process gracefully exited before 1s grace period\" \n",
			expectedMarker: strconv.Itoa(InternalErrorCode),
		},
		{
			name:           "command times out and ignores interrupt",
			args:           []string{"bash", "-c", "trap 'sleep 10' EXIT; sleep 10"},
			timeout:        1 * time.Second,
			gracePeriod:    1 * time.Second,
			expectedLog:    "level=error msg=\"Process did not finish before 1s timeout\" \nlevel=error msg=\"Process did not exit before 1s grace period\" \n",
			expectedMarker: strconv.Itoa(InternalErrorCode),
		},
		{
			// Ensure that environment variables get passed through
			name:           "$PATH is set",
			args:           []string{"sh", "-c", "echo $PATH"},
			expectedLog:    os.Getenv("PATH") + "\n",
			expectedMarker: "0",
		},
	}

	// we write logs to the process log if wrapping fails
	// and cannot write timestamps or we can't match text
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", testCase.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", testCase.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", testCase.name, err)
				}
			}()

			options := Options{
				Timeout:     testCase.timeout,
				GracePeriod: testCase.gracePeriod,
				Options: &wrapper.Options{
					Args:       testCase.args,
					ProcessLog: path.Join(tmpDir, "process-log.txt"),
					MarkerFile: path.Join(tmpDir, "marker-file.txt"),
				},
			}

			if code := strconv.Itoa(options.Run()); code != testCase.expectedMarker {
				t.Errorf("%s: exit code %q does not match expected marker file contents %q", testCase.name, code, testCase.expectedMarker)
			}

			compareFileContents(testCase.name, options.ProcessLog, testCase.expectedLog, t)
			compareFileContents(testCase.name, options.MarkerFile, testCase.expectedMarker, t)
		})
	}
}

func compareFileContents(name, file, expected string, t *testing.T) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: could not read file: %v", name, err)
	}
	if string(data) != expected {
		t.Errorf("%s: expected contents: %q, got %q", name, expected, data)
	}
}
