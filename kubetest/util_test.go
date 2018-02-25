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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestHttpFileScheme(t *testing.T) {
	expected := "some testdata"
	tmpfile, err := ioutil.TempFile("", "test_http_file_scheme")
	if err != nil {
		t.Errorf("Error creating temporary file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.WriteString(expected); err != nil {
		t.Errorf("Error writing to temporary file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Errorf("Error closing temporary file: %v", err)
	}

	fileURL := fmt.Sprintf("file://%s", tmpfile.Name())
	buf := new(bytes.Buffer)
	if err := httpRead(fileURL, buf); err != nil {
		t.Errorf("Error reading temporary file through httpRead: %v", err)
	}

	if buf.String() != expected {
		t.Errorf("httpRead(%s): expected %v, got %v", fileURL, expected, buf)
	}
}

func TestGetLatestClusterUpTime(t *testing.T) {
	const magicTime = "2011-11-11T11:11:11.111-11:00"
	myTime, err := time.Parse(time.RFC3339, magicTime)
	if err != nil {
		t.Fatalf("Fail parsing time: %v", err)
	}

	cases := []struct {
		name         string
		body         string
		expectedTime time.Time
		expectErr    bool
	}{
		{
			name:      "bad json",
			body:      "abc",
			expectErr: true,
		},
		{
			name:         "empty json",
			body:         "[]",
			expectedTime: time.Time{},
		},
		{
			name:         "valid json",
			body:         "[{\"name\": \"foo\", \"creationTimestamp\": \"2011-11-11T11:11:11.111-11:00\"}]",
			expectedTime: myTime,
		},
		{
			name:      "bad time format",
			body:      "[{\"name\": \"foo\", \"creationTimestamp\": \"blah-blah\"}]",
			expectErr: true,
		},
		{
			name:         "multiple entries",
			body:         "[{\"name\": \"foo\", \"creationTimestamp\": \"2011-11-11T11:11:11.111-11:00\"}, {\"name\": \"bar\", \"creationTimestamp\": \"2010-10-10T11:11:11.111-11:00\"}]",
			expectedTime: myTime,
		},
	}
	for _, tc := range cases {
		time, err := getLatestClusterUpTime(tc.body)
		if err != nil && !tc.expectErr {
			t.Errorf("%s: got unexpected error %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expect error but did not get one", tc.name)
		}
		if !tc.expectErr && !time.Equal(tc.expectedTime) {
			t.Errorf("%s: expect time %v, but got %v", tc.name, tc.expectedTime, time)
		}
	}
}
