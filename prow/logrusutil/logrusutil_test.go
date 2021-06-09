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

package logrusutil

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestCensoringFormatter(t *testing.T) {

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
	formatter := NewCensoringFormatter(baseFormatter, func() sets.String {
		return sets.NewString("MYSTERY", "SECRET")
	})

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

func TestCensoringFormatterWithCornerCases(t *testing.T) {
	entry := &logrus.Entry{Message: "message", Data: logrus.Fields{"key": fmt.Errorf("A SECRET is a secret")}}
	expectedEntry := "level=panic msg=message key=\"A ****** is a secret\"\n"

	testCases := []struct {
		description string
		secrets     sets.String
		expected    string
	}{
		{
			description: "empty string",
			secrets:     sets.NewString("SECRET", ""),
			expected:    expectedEntry,
		},
		{
			description: "leading line break",
			secrets:     sets.NewString("\nSECRET", ""),
			expected:    expectedEntry,
		},
		{
			description: "tailing line break",
			secrets:     sets.NewString("SECRET\n", ""),
			expected:    expectedEntry,
		},
		{
			description: "leading space and tailing space",
			secrets:     sets.NewString(" SECRET ", ""),
			expected:    expectedEntry,
		},
	}

	baseFormatter := &logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			formatter := NewCensoringFormatter(baseFormatter, func() sets.String {
				return tc.secrets
			})

			censored, err := formatter.Format(entry)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if string(censored) != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, string(censored))
			}
		})
	}
}

func TestCensoringFormatterDoesntDeadLockWhenUsedWithStandardLogger(t *testing.T) {
	// The whitespace makes the censoring fornmatter emit a warning. If it uses the same global
	// logger, that results in a deadlock.
	logrus.SetFormatter(NewCensoringFormatter(logrus.StandardLogger().Formatter, func() sets.String {
		return sets.NewString(" untrimmed")
	}))
	logrus.Info("test")
}
