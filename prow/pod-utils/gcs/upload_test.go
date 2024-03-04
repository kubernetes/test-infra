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
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	stdio "io"
	"os"
	"path"
	"reflect"
	"sync"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
)

type (
	fakeReader struct {
		closeWillFail bool
		meta          *readerFuncMetadata
	}
	readerFuncMetadata struct {
		NewReaderAttemptsNum int
		ReaderWasClosed      bool
	}
	readerFuncOptions struct {
		newAlwaysFail          bool
		newFailsOnNthAttempt   int
		closeFailsOnNthAttempt int
		closeAlwaysFails       bool
	}
)

func (r *fakeReader) Read(p []byte) (n int, err error) {
	return 0, stdio.EOF
}

func (r *fakeReader) Close() error {
	if r.closeWillFail {
		return errors.New("fake reader: close fails")
	}
	r.meta.ReaderWasClosed = true
	return nil
}

func newReaderFunc(opt readerFuncOptions) (ReaderFunc, *readerFuncMetadata) {
	meta := readerFuncMetadata{}
	return ReaderFunc(func() (io.ReadCloser, error) {
		defer func() {
			meta.NewReaderAttemptsNum += 1
		}()
		if opt.newAlwaysFail {
			return nil, errors.New("reader func: always failing")
		}
		if opt.newFailsOnNthAttempt > -1 && meta.NewReaderAttemptsNum == opt.newFailsOnNthAttempt {
			return nil, fmt.Errorf("reader func: fails on attempt no.: %d", meta.NewReaderAttemptsNum)
		}
		closeWillFail := opt.closeAlwaysFails
		if opt.closeFailsOnNthAttempt > -1 && meta.NewReaderAttemptsNum == opt.closeFailsOnNthAttempt {
			closeWillFail = true
		}
		return &fakeReader{closeWillFail: closeWillFail, meta: &meta}, nil
	}), &meta
}

func TestUploadNewReaderFunc(t *testing.T) {
	var testCases = []struct {
		name               string
		compressFileTypes  []string
		isErrExpected      bool
		readerFuncOpts     readerFuncOptions
		wantReaderFuncMeta readerFuncMetadata
	}{
		{
			name:          "Succeed on first retry",
			isErrExpected: false,
			readerFuncOpts: readerFuncOptions{
				newFailsOnNthAttempt:   -1,
				closeFailsOnNthAttempt: -1,
			},
			wantReaderFuncMeta: readerFuncMetadata{
				NewReaderAttemptsNum: 1,
				ReaderWasClosed:      true,
			},
		},
		{
			name:          "Reader cannot be created",
			isErrExpected: true,
			readerFuncOpts: readerFuncOptions{
				newAlwaysFail:          true,
				newFailsOnNthAttempt:   -1,
				closeFailsOnNthAttempt: -1,
			},
			wantReaderFuncMeta: readerFuncMetadata{
				NewReaderAttemptsNum: 4,
				ReaderWasClosed:      false,
			},
		},
		{
			name: "Fail on first attempt",
			readerFuncOpts: readerFuncOptions{
				newFailsOnNthAttempt:   0,
				closeFailsOnNthAttempt: -1,
			},
			wantReaderFuncMeta: readerFuncMetadata{
				NewReaderAttemptsNum: 2,
				ReaderWasClosed:      true,
			},
		},
		{
			name:              "Compress files",
			compressFileTypes: []string{"*"},
			readerFuncOpts: readerFuncOptions{
				newFailsOnNthAttempt:   -1,
				closeFailsOnNthAttempt: -1,
			},
			wantReaderFuncMeta: readerFuncMetadata{
				NewReaderAttemptsNum: 1,
				ReaderWasClosed:      true,
			},
		},
	}
	tempDir := t.TempDir()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := os.CreateTemp(tempDir, "*test-upload-new-read-fn")
			if err != nil {
				t.Fatalf("create tmp file: %v", err)
			}
			uploadTargets := make(map[string]UploadFunc)
			readerFunc, readerFuncMeta := newReaderFunc(testCase.readerFuncOpts)
			uploadTargets[path.Base(f.Name())] = DataUpload(readerFunc)
			bucket := fmt.Sprintf("%s://%s", providers.File, path.Dir(f.Name()))
			err = Upload(context.TODO(), bucket, "", "", testCase.compressFileTypes, uploadTargets)
			if testCase.isErrExpected && err == nil {
				t.Errorf("error expected but got nil")
			}
			if !reflect.DeepEqual(testCase.wantReaderFuncMeta, *readerFuncMeta) {
				t.Errorf("unexpected ReaderFuncMetadata: %s", diff.ObjectReflectDiff(testCase.wantReaderFuncMeta, *readerFuncMeta))
			}
		})
	}

}

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
		t.Run(testCase.name, func(t *testing.T) {

			uploadFuncs := map[string]UploadFunc{}

			currentTestStates := map[string]destUploadBehavior{}
			currentTestStatesLock := sync.Mutex{}

			for _, destBehavior := range testCase.destUploadBehaviors {

				currentTestStates[destBehavior.dest] = destBehavior

				uploadFuncs[destBehavior.dest] = func(destBehavior destUploadBehavior) UploadFunc {

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
				}(destBehavior)

			}

			ctx := context.Background()
			err := Upload(ctx, "", "", "", []string{}, uploadFuncs)

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

			if (err != nil) != isErrExpected {
				t.Errorf("%v: Got unexpected error response: %v", testCase.name, err)
			}
		})

	}
}

func TestUploadCompression(t *testing.T) {
	type destUploadBehavior struct {
		dest     string
		compress bool
	}

	testCases := []struct {
		name               string
		destUploadBehavior []destUploadBehavior
		compressFileTypes  []string
	}{
		{
			name: "compress all",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/log.txt",
					compress: true,
				},
				{
					dest:     "testdata/graph.json",
					compress: true,
				},
			},
			compressFileTypes: []string{"*"},
		},
		{
			name: "compress txt only",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/log.txt",
					compress: true,
				},
				{
					dest:     "testdata/graph.json",
					compress: false,
				},
			},
			compressFileTypes: []string{"txt"},
		},
		{
			name: "compress multiple types, but some others included",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/log.txt",
					compress: true,
				},
				{
					dest:     "testdata/graph.json",
					compress: false,
				},
				{
					dest:     "testdata/something.log",
					compress: true,
				},
			},
			compressFileTypes: []string{"txt", "log"},
		},
		{
			name: "compress all, ignores small file",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/small.json",
					compress: false,
				},
			},
			compressFileTypes: []string{"*"},
		},
		{
			name: "compress all, ignores gz and gzip file",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/something.gz",
					compress: false,
				},
				{
					dest:     "testdata/something.gzip",
					compress: false,
				},
			},
			compressFileTypes: []string{"*"},
		},
		{
			name: "compress log, ignores file that is gzipped despite having log extension",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/really-a-zip.log",
					compress: false,
				},
			},
			compressFileTypes: []string{"log"},
		},
		{
			name: "compress nothing",
			destUploadBehavior: []destUploadBehavior{
				{
					dest:     "testdata/log.txt",
					compress: false,
				},
				{
					dest:     "testdata/graph.json",
					compress: false,
				},
			},
			compressFileTypes: []string{},
		},
	}
	for _, tc := range testCases {
		uploadFuncs := map[string]UploadFunc{}
		for _, destBehavior := range tc.destUploadBehavior {
			uploadFuncs[destBehavior.dest] = func(destBehavior destUploadBehavior) UploadFunc {
				return func(writer dataWriter) error {
					if destBehavior.compress != writer.compressData() {
						t.Errorf("expected upload to be compressed=%v, but writer wasn't set up correctly for dest: %s", destBehavior.compress, destBehavior.dest)
					}
					return nil
				}
			}(destBehavior)
		}
		t.Run(tc.name, func(t *testing.T) {
			err := Upload(context.Background(), "", "", "", tc.compressFileTypes, uploadFuncs)
			if err != nil {
				t.Errorf("unexpected error returned from Upload: %v", err)
			}

		})
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
		compress      bool
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
		{
			name:          "compress file",
			ObjectDest:    "build/log.text",
			ObjectContent: []byte("Oh wow\nlogs\nthis is\ncrazy"),
			compress:      true,
			wantN:         25,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &openerObjectWriter{
				Opener:   io.NewGCSOpener(fakeGCSClient),
				Context:  context.Background(),
				Bucket:   fmt.Sprintf("gs://%s", fakeBucket),
				Dest:     tt.ObjectDest,
				compress: tt.compress,
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

			var gotObjectContent []byte
			if tt.compress {
				gzipReader, err := gzip.NewReader(reader)
				if err != nil {
					t.Errorf("Got unexpected error creating gzipReader for object %s: %v", tt.ObjectDest, err)
				}
				gotObjectContent, err = stdio.ReadAll(gzipReader)
				if err != nil {
					t.Errorf("Got unexpected error reading object %s: %v", tt.ObjectDest, err)
				}
			} else {
				gotObjectContent, err = stdio.ReadAll(reader)
				if err != nil {
					t.Errorf("Got unexpected error reading object %s: %v", tt.ObjectDest, err)
				}
			}

			if !bytes.Equal(tt.ObjectContent, gotObjectContent) {
				t.Errorf("Write() gotObjectContent = %v, want %v", gotObjectContent, tt.ObjectContent)
			}
		})
	}
}

func Test_openerObjectWriter_fullUploadPath(t *testing.T) {
	tests := []struct {
		name   string
		bucket string
		dest   string
		want   string
	}{
		{
			name:   "simple path",
			bucket: "bucket-A",
			dest:   "path/to/some/file.json",
			want:   "gs://bucket-A/path/to/some/file.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &openerObjectWriter{
				Bucket: fmt.Sprintf("gs://%s", tt.bucket),
				Dest:   tt.dest,
			}
			got := w.fullUploadPath()

			if got != tt.want {
				t.Errorf("fullUploadPath(): got %v, want %v", got, tt.want)
				return
			}
		})
	}
}
