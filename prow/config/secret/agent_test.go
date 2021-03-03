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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/logrusutil"
)

func TestCensoringFormatter(t *testing.T) {
	var err error
	secret1, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("failed to set up a temporary file: %v", err)
	}
	if _, err := secret1.WriteString("SECRET"); err != nil {
		t.Fatalf("failed to write a fake secret to a file: %v", err)
	}
	defer secret1.Close()
	defer os.Remove(secret1.Name())
	secret2, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("failed to set up a temporary file: %v", err)
	}
	if _, err := secret2.WriteString("MYSTERY"); err != nil {
		t.Fatalf("failed to write a fake secret to a file: %v", err)
	}
	defer secret2.Close()
	defer os.Remove(secret2.Name())

	agent := Agent{}
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
			expected:    "level=panic msg=\"A ****** is a ****** if it is secret\"\n",
		},
		{
			description: "occurrences of a multiple secrets in a message are censored",
			entry:       &logrus.Entry{Message: "A SECRET is a MYSTERY"},
			expected:    "level=panic msg=\"A ****** is a *******\"\n",
		},
		{
			description: "occurrences of multiple secrets in a field",
			entry:       &logrus.Entry{Message: "message", Data: logrus.Fields{"key": "A SECRET is a MYSTERY"}},
			expected:    "level=panic msg=message key=\"A ****** is a *******\"\n",
		},
		{
			description: "occurrences of a secret in a non-string field",
			entry:       &logrus.Entry{Message: "message", Data: logrus.Fields{"key": fmt.Errorf("A SECRET is a MYSTERY")}},
			expected:    "level=panic msg=message key=\"A ****** is a *******\"\n",
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
