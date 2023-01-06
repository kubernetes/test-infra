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

package gcs

import (
	"mime"
	"reflect"
	"testing"

	utilpointer "k8s.io/utils/pointer"

	"k8s.io/test-infra/prow/io"
)

func TestWriterOptionsFromFileName(t *testing.T) {
	mime.AddExtensionType(".log", "text/plain")

	testCases := []struct {
		name             string
		filename         string
		expectedFileName string
		expectedAttrs    io.WriterOptions
	}{
		{
			name:             "txt",
			filename:         "build-log.txt",
			expectedFileName: "build-log.txt",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("text/plain; charset=utf-8"),
			},
		},
		{
			name:             "txt.gz",
			filename:         "build-log.txt.gz",
			expectedFileName: "build-log.txt",
			expectedAttrs: io.WriterOptions{
				ContentEncoding: utilpointer.StringPtr("gzip"),
				ContentType:     utilpointer.StringPtr("text/plain; charset=utf-8"),
			},
		},
		{
			name:             "txt.gzip",
			filename:         "build-log.txt.gzip",
			expectedFileName: "build-log.txt",
			expectedAttrs: io.WriterOptions{
				ContentEncoding: utilpointer.StringPtr("gzip"),
				ContentType:     utilpointer.StringPtr("text/plain; charset=utf-8"),
			},
		},
		{
			name:             "bare gz",
			filename:         "gz",
			expectedFileName: "gz",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("application/gzip"),
			},
		},
		{
			name:             "gz",
			filename:         "build-log.gz",
			expectedFileName: "build-log",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("application/gzip"),
			},
		},
		{
			name:             "gzip",
			filename:         "build-log.gzip",
			expectedFileName: "build-log",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("application/gzip"),
			},
		},
		{
			name:             "json",
			filename:         "events.json",
			expectedFileName: "events.json",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("application/json"),
			},
		},
		{
			name:             "json.gz",
			filename:         "events.json.gz",
			expectedFileName: "events.json",
			expectedAttrs: io.WriterOptions{
				ContentEncoding: utilpointer.StringPtr("gzip"),
				ContentType:     utilpointer.StringPtr("application/json"),
			},
		},
		{
			name:             "log",
			filename:         "journal.log",
			expectedFileName: "journal.log",
			expectedAttrs: io.WriterOptions{
				ContentType: utilpointer.StringPtr("text/plain; charset=utf-8"),
			},
		},
		{
			name:             "empty",
			filename:         "",
			expectedFileName: "",
			expectedAttrs:    io.WriterOptions{},
		},
		{
			name:             "dot",
			filename:         ".",
			expectedFileName: ".",
			expectedAttrs:    io.WriterOptions{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			actualFileName, actualAttrs := WriterOptionsFromFileName(test.filename)

			if actualFileName != test.expectedFileName {
				t.Errorf("expected file name %q but got %q", test.expectedFileName, actualFileName)
			}

			if !reflect.DeepEqual(actualAttrs, test.expectedAttrs) {
				t.Errorf("expected attributes %#+v but got %#+v", test.expectedAttrs, actualAttrs)
			}
		})
	}
}
