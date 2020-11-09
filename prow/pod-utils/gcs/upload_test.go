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
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"k8s.io/test-infra/prow/io"
)

func TestUploadWithRetries(t *testing.T) {

	// doesPass = true, isFlaky = false => Pass in first attempt
	// doesPass = true, isFlaky = true => Pass in second attempt
	// doesPass = false, isFlaky = don't care => Fail to upload in all attempts
	type destUploadBehavior struct {
		dest     string
		isFlaky  bool
		doesPass bool
	}

	var testCases = []struct {
		name                string
		destUploadBehaviors []destUploadBehavior
	}{
		{
			name: "all passed",
			destUploadBehaviors: []destUploadBehavior{
				{
					dest:     "all-pass-dest1",
					doesPass: true,
					isFlaky:  false,
				},
				{
					dest:     "all-pass-dest2",
					doesPass: true,
					isFlaky:  false,
				},
			},
		},
		{
			name: "all passed with retries",
			destUploadBehaviors: []destUploadBehavior{
				{
					dest:     "all-pass-retries-dest1",
					doesPass: true,
					isFlaky:  true,
				},
				{
					dest:     "all-pass-retries-dest2",
					doesPass: true,
					isFlaky:  false,
				},
			},
		},
		{
			name: "all failed",
			destUploadBehaviors: []destUploadBehavior{
				{
					dest:     "all-failed-dest1",
					doesPass: false,
					isFlaky:  false,
				},
				{
					dest:     "all-failed-dest2",
					doesPass: false,
					isFlaky:  false,
				},
			},
		},
		{
			name: "some failed",
			destUploadBehaviors: []destUploadBehavior{
				{
					dest:     "some-failed-dest1",
					doesPass: true,
					isFlaky:  false,
				},
				{
					dest:     "some-failed-dest2",
					doesPass: false,
					isFlaky:  false,
				},
			},
		},
	}

	for _, testCase := range testCases {

		uploadFuncs := map[string]UploadFunc{}

		currentTestStates := map[string]destUploadBehavior{}
		currentTestStatesLock := sync.Mutex{}

		for _, destBehavior := range testCase.destUploadBehaviors {

			currentTestStates[destBehavior.dest] = destBehavior

			getUploadFunc := func(destBehavior destUploadBehavior) UploadFunc {

				return func(writer dataWriter) error {
					currentTestStatesLock.Lock()
					defer currentTestStatesLock.Unlock()

					currentDestUploadBehavior := currentTestStates[destBehavior.dest]

					if !currentDestUploadBehavior.doesPass {
						return fmt.Errorf("%v: %v failed", testCase.name, destBehavior.dest)
					}

					if currentDestUploadBehavior.isFlaky {
						currentDestUploadBehavior.isFlaky = false
						currentTestStates[destBehavior.dest] = currentDestUploadBehavior
						return fmt.Errorf("%v: %v flaky", testCase.name, destBehavior.dest)
					}

					delete(currentTestStates, destBehavior.dest)
					return nil
				}
			}

			uploadFuncs[destBehavior.dest] = getUploadFunc(destBehavior)

		}

		err := Upload("", "", "", uploadFuncs)

		isErrExpected := false
		for _, currentTestState := range currentTestStates {

			if currentTestState.doesPass {
				t.Errorf("%v: %v did not get uploaded", testCase.name, currentTestState.dest)
				break
			}

			if !isErrExpected && !currentTestState.doesPass {
				isErrExpected = true
			}
		}

		if err != nil && !isErrExpected {
			t.Errorf("%v: Got unexpected error response: %v", testCase.name, err)
		}
	}
}

func Test_openerObjectWriter_Write(t *testing.T) {

	fakeBucket := "test-bucket"
	fakeGCSServer := fakestorage.NewServer([]fakestorage.Object{})
	fakeGCSServer.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: fakeBucket})
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

			if !bytes.Equal(tt.ObjectContent, gotObjectContent) {
				t.Errorf("Write() gotObjectContent = %v, want %v", gotObjectContent, tt.ObjectContent)
			}
		})
	}
}
