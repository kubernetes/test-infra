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

package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/logrusutil"
)

func TestCensoringFormatter(t *testing.T) {
	var err error
	secret1, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("failed to set up a temporary file: %v", err)
	}
	if _, err := secret1.WriteString("SECRET"); err != nil {
		t.Fatalf("failed to write a fake secret to a file: %v", err)
	}
	defer secret1.Close()
	defer os.Remove(secret1.Name())
	secret2, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("failed to set up a temporary file: %v", err)
	}
	if _, err := secret2.WriteString("MYSTERY"); err != nil {
		t.Fatalf("failed to write a fake secret to a file: %v", err)
	}
	defer secret2.Close()
	defer os.Remove(secret2.Name())

	agent := agent{}
	if err = agent.Start([]string{secret1.Name(), secret2.Name()}); err != nil {
		t.Fatalf("failed to start a secret agent: %v", err)
	}

	testCases := []struct {
		description string
		entry       *logrus.Entry
		expected    string
	}{
		{
			description: "all occurrences of a single secret in a message are censored",
			entry:       &logrus.Entry{Message: "A SECRET is a SECRET if it is secret"},
			expected:    "level=panic msg=\"A XXXXXX is a XXXXXX if it is secret\"\n",
		},
		{
			description: "occurrences of a multiple secrets in a message are censored",
			entry:       &logrus.Entry{Message: "A SECRET is a MYSTERY"},
			expected:    "level=panic msg=\"A XXXXXX is a XXXXXXX\"\n",
		},
		{
			description: "occurrences of multiple secrets in a field",
			entry:       &logrus.Entry{Message: "message", Data: logrus.Fields{"key": "A SECRET is a MYSTERY"}},
			expected:    "level=panic msg=message key=\"A XXXXXX is a XXXXXXX\"\n",
		},
		{
			description: "occurrences of a secret in a non-string field",
			entry:       &logrus.Entry{Message: "message", Data: logrus.Fields{"key": fmt.Errorf("A SECRET is a MYSTERY")}},
			expected:    "level=panic msg=message key=\"A XXXXXX is a XXXXXXX\"\n",
		},
	}

	baseFormatter := &logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	}
	formatter := logrusutil.NewCensoringFormatter(baseFormatter, agent.getSecrets)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			censored, err := formatter.Format(tc.entry)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if string(censored) != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, string(censored))
			}
		})
	}
}

func TestAddWithParser(t *testing.T) {
	t.Parallel()
	// Go never runs a test in parallel with itself, so run
	// the test twice to make sure the race detector checks
	// the thread safety.
	for idx := range []int{0, 1} {
		t.Run(strconv.Itoa(idx), testAddWithParser)
	}
}

func testAddWithParser(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	secretPath := filepath.Join(tmpDir, "secret")
	if err := os.WriteFile(secretPath, []byte("1"), 0644); err != nil {
		t.Fatalf("failed to write initial content of secret: %v", err)
	}

	vals := make(chan int, 3)
	errs := make(chan error, 3)
	generator, err := AddWithParser(
		secretPath,
		func(raw []byte) (int, error) {
			val, err := strconv.Atoi(string(raw))
			if err != nil {
				errs <- err
				return val, err
			}
			vals <- val
			return val, err
		},
	)
	if err != nil {
		t.Fatalf("AddWithParser failed: %v", err)
	}

	checkValueAndErr := func(expected int, e error) {
		t.Helper()
		select {
		case v := <-vals:
			if v != expected {
				t.Errorf("expected value to get updated to %d but got updated to %d", expected, v)
			}
			break
		case err := <-errs:
			if e == nil || e.Error() != err.Error() {
				t.Fatalf("expected error %v, got %v", e, err)
			}
		// the agent reloads every second, so ten seconds should be plenty
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for value %d and error %d", expected, e)
		}
		if actual := generator(); actual != expected {
			t.Errorf("expected value %d from generator, got %d", expected, actual)
		}
	}
	checkValueAndErr(1, nil)

	if err := os.WriteFile(secretPath, []byte("2"), 0644); err != nil {
		t.Fatalf("failed to update secret on disk: %v", err)
	}
	// expect secret to get updated
	checkValueAndErr(2, nil)

	if err := os.WriteFile(secretPath, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("failed to update secret on disk: %v", err)
	}
	// expect secret to remain unchanged and an error in the parsing func
	checkValueAndErr(2, errors.New(`strconv.Atoi: parsing "not-a-number": invalid syntax`))
}
