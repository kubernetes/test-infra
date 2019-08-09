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

package gcs

import (
	"mime"
	"strings"

	"cloud.google.com/go/storage"

	"k8s.io/test-infra/testgrid/metadata"
)

// TODO(fejta): migrate usage off type alias.

// Started holds started.json data
type Started = metadata.Started

// Finished holds finished.json data
type Finished = metadata.Finished

// AttributesFromFileName guesses file attributes from the filename
// and returns the attributes and a simplifed filename.  For example,
// build-log.txt.gz would be:
//
//   Content-Type: text/plain; charset=utf-8
//   Content-Encoding: gzip
//
// and the simplified filename would be build-log.txt (excluding the
// content encoding extension).
func AttributesFromFileName(filename string) (string, *storage.ObjectAttrs) {
	attrs := &storage.ObjectAttrs{}
	segments := strings.Split(filename, ".")
	index := len(segments) - 1
	segment := segments[index]

	// https://www.iana.org/assignments/http-parameters/http-parameters.xhtml#content-coding
	switch segment {
	case "gz", "gzip":
		attrs.ContentEncoding = "gzip"
	}

	if attrs.ContentEncoding != "" {
		if index == 0 {
			segment = ""
		} else {
			filename = filename[:len(filename)-len(segment)-1]
			index -= 1
			segment = segments[index]
		}
	}

	if segment != "" {
		mediaType := mime.TypeByExtension("." + segment)
		if mediaType != "" {
			attrs.ContentType = mediaType
		}
	}

	if attrs.ContentType == "" && attrs.ContentEncoding == "gzip" {
		attrs.ContentType = "application/gzip"
		attrs.ContentEncoding = ""
	}

	return filename, attrs
}
