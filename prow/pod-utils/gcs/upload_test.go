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

package gcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"k8s.io/test-infra/prow/io"
)

func TestUploadToGcs(t *testing.T) {
	var testCases = []struct {
		name           string
		passingTargets int
		failingTargets int
		expectedErr    bool
	}{
		{
			name:           "all passing",
			passingTargets: 10,
			failingTargets: 0,
			expectedErr:    false,
		},
		{
			name:           "all but one passing",
			passingTargets: 10,
			failingTargets: 1,
			expectedErr:    true,
		},
		{
			name:           "all but one failing",
			passingTargets: 1,
			failingTargets: 10,
			expectedErr:    true,
		},
		{
			name:           "all failing",
			passingTargets: 0,
			failingTargets: 10,
			expectedErr:    true,
		},
	}

	for _, testCase := range testCases {
		lock := sync.Mutex{}
		count := 0

		update := func() {
			lock.Lock()
			defer lock.Unlock()
			count = count + 1
		}

		fail := func(_ dataWriter) error {
			update()
			return errors.New("fail")
		}

		success := func(_ dataWriter) error {
			update()
			return nil
		}

		targets := map[string]UploadFunc{}
		for i := 0; i < testCase.passingTargets; i++ {
			targets[fmt.Sprintf("pass-%d", i)] = success
		}

		for i := 0; i < testCase.failingTargets; i++ {
			targets[fmt.Sprintf("fail-%d", i)] = fail
		}

		err := Upload("", "", "", targets)
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got %v", testCase.name, err)
		}
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}

		if count != (testCase.passingTargets + testCase.failingTargets) {
			t.Errorf("%s: had %d passing and %d failing targets but only ran %d targets, not %d", testCase.name, testCase.passingTargets, testCase.failingTargets, count, testCase.passingTargets+testCase.failingTargets)
		}
	}
}

func Test_openerObjectWriter_Write(t *testing.T) {

	fakeBucket := "test-bucket"
	fakeGCSServer := fakestorage.NewServer([]fakestorage.Object{})
	fakeGCSServer.CreateBucket(fakeBucket)
	defer fakeGCSServer.Stop()
	fakeGCSClient := fakeGCSServer.Client()

	tests := []struct {
		name          string
		ObjectDest    string
		ObjectContent []byte
		wantN         int
		wantErr       bool
	}{
		{
			name:          "write regular file",
			ObjectDest:    "build/log.text",
			ObjectContent: []byte("Oh wow\nlogs\nthis is\ncrazy"),
			wantN:         25,
			wantErr:       false,
		},
		{
			name:          "write empty file",
			ObjectDest:    "build/marker",
			ObjectContent: []byte(""),
			wantN:         0,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &openerObjectWriter{
				Opener:  io.NewGCSOpener(fakeGCSClient),
				Context: context.Background(),
				Bucket:  fmt.Sprintf("gs://%s", fakeBucket),
				Dest:    tt.ObjectDest,
			}
			gotN, err := w.Write(tt.ObjectContent)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("Write() gotN = %v, want %v", gotN, tt.wantN)
			}

			if err := w.Close(); (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// read object back from bucket and compare with written object
			reader, err := fakeGCSClient.Bucket(fakeBucket).Object(tt.ObjectDest).NewReader(context.Background())
			if err != nil {
				t.Errorf("Got unexpected error reading object %s: %v", tt.ObjectDest, err)
			}

			gotObjectContent, err := ioutil.ReadAll(reader)
			if err != nil {
				t.Errorf("Got unexpected error reading object %s: %v", tt.ObjectDest, err)
			}

			if bytes.Compare(tt.ObjectContent, gotObjectContent) != 0 {
				t.Errorf("Write() gotObjectContent = %v, want %v", gotObjectContent, tt.ObjectContent)
			}
		})
	}
}
