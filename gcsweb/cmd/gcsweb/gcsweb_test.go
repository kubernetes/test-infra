/*
Copyright 2016 The Kubernetes Authors.

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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/fakeopener"
)

type fakeOpener struct {
	fakeopener.FakeOpener

	objects     []gcsObject
	directories []string
}

func newFakeOpener(objects []gcsObject, directories []string) *fakeOpener {
	f := &fakeOpener{
		FakeOpener: fakeopener.FakeOpener{
			Buffer: make(map[string]*bytes.Buffer),
		},
		objects:     objects,
		directories: directories,
	}

	for _, object := range objects {
		f.Buffer[joinPath(object.BucketName, object.Name)] = bytes.NewBuffer(object.Content)
	}

	return f
}

func (f *fakeOpener) Iterator(ctx context.Context, prefix string, delimiter string) (pkgio.ObjectIterator, error) {
	prowPath, err := prowv1.ParsePath(prefix)
	if err != nil {
		return nil, err
	}
	bucket := prowPath.BucketWithScheme()

	var objects []pkgio.ObjectAttributes

	for _, object := range f.objects {
		if object.BucketName == bucket && filepath.Dir(object.Name)+"/" == prowPath.Path {
			objects = append(objects, pkgio.ObjectAttributes{
				Name:    joinPath(bucket, object.Name),
				ObjName: filepath.Base(object.Name),
				Size:    int64(len(object.Content)),
				Updated: object.Updated,
			})
		}
	}

	for _, directory := range f.directories {
		objects = append(objects, pkgio.ObjectAttributes{
			Name:  joinPath(bucket, directory),
			IsDir: true,
		})
	}

	return &fakeIterator{objects: objects}, nil
}

type fakeIterator struct {
	i       int
	objects []pkgio.ObjectAttributes
}

func (f *fakeIterator) Next(ctx context.Context) (attr pkgio.ObjectAttributes, err error) {
	if f.i >= len(f.objects) {
		return pkgio.ObjectAttributes{}, io.EOF
	}

	defer func() { f.i++ }()
	return f.objects[f.i], nil
}

type gcsObject struct {
	BucketName string    `json:"-"`
	Name       string    `json:"name"`
	Content    []byte    `json:"-"`
	Updated    time.Time `json:"updated,omitempty"`
}

func TestHandleObject(t *testing.T) {
	testCases := []struct {
		id              string
		initialObjects  []gcsObject
		path            string
		headers         objectHeaders
		expected        string
		expectedHeaders http.Header
		errorExpected   bool
	}{
		{
			id:   "happy GCS case",
			path: "/gcs/test-bucket/path/to/file1",
			initialObjects: []gcsObject{
				{
					BucketName: "gs://test-bucket",
					Name:       "path/to/file1",
					Content:    []byte("123456789"),
				},
			},
			expectedHeaders: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			expected:        "123456789",
		},
		{
			id:   "happy S3 case",
			path: "/s3/test-bucket/path/to/file1",
			initialObjects: []gcsObject{
				{
					BucketName: "s3://test-bucket",
					Name:       "path/to/file1",
					Content:    []byte("123456789"),
				},
			},
			expectedHeaders: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			expected:        "123456789",
		},
		{
			id:              "sad case",
			path:            "/gcs/test-bucket/path/to/unknown",
			expectedHeaders: http.Header{},
			errorExpected:   true,
		},
		{
			id:   "happy html case",
			path: "/gcs/test-bucket/path/to/file1",
			initialObjects: []gcsObject{
				{
					BucketName: "gs://test-bucket",
					Name:       "path/to/file1",
					Content: []byte(`
<!doctype html>
<html>
  <head>
    <title>Test Title</title>
  </head>
  <body>
    My Test Body
  </body>
</html>`),
				},
			},
			headers: objectHeaders{
				contentType:        "text/html",
				contentEncoding:    "UTF-8",
				contentDisposition: "inline",
				contentLanguage:    "en-US",
			},
			expectedHeaders: http.Header{"Content-Disposition": []string{"inline"}, "Content-Language": []string{"en-US"}, "Content-Type": []string{"text/html; charset=UTF-8"}},
			expected: `
<!doctype html>
<html>
  <head>
    <title>Test Title</title>
  </head>
  <body>
    My Test Body
  </body>
</html>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			w := httptest.NewRecorder()
			s := server{storageClient: newFakeOpener(tc.initialObjects, nil)}

			prowPath, err := parsePath(tc.path)
			if err != nil {
				t.Fatal(err)
			}

			err = s.handleObject(w, prowPath, tc.headers)
			if err != nil && !tc.errorExpected {
				t.Fatalf("Error not expected: %v", err)
			}

			if err == nil && tc.errorExpected {
				t.Fatalf("Error was expected")
			}
			actualHeaders := w.Result().Header
			actualBody := w.Body.String()

			if !reflect.DeepEqual(actualHeaders, tc.expectedHeaders) {
				t.Fatal(cmp.Diff(actualHeaders, tc.expectedHeaders))
			}

			if !reflect.DeepEqual(actualBody, tc.expected) {
				t.Fatal(cmp.Diff(actualBody, tc.expected))
			}
		})
	}
}

func TestHandleDirectory(t *testing.T) {
	testCases := []struct {
		id             string
		initialDirs    []string
		initialObjects []gcsObject
		path           string
		expected       string
	}{
		{
			id:          "happy GCS case",
			path:        "/gcs/test-bucket/pr-logs/12345/",
			initialDirs: []string{"/1", "/2"},
			initialObjects: []gcsObject{
				{
					BucketName: "gs://test-bucket",
					Name:       "/pr-logs/12345/file1",
					Updated:    time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "gs://test-bucket",
					Name:       "/pr-logs/12345/file2",
					Updated:    time.Date(2000, time.January, 1, 22, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "gs://test-bucket",
					Name:       "/pr-logs/12345/file3",
					Updated:    time.Date(2000, time.January, 1, 23, 0, 0, 0, time.UTC),
				},
			},
			expected: `
    <!doctype html>
   	<html>
   	<head>
   	    <link rel="stylesheet" type="text/css" href="/styles/style.css">
   	    <meta charset="utf-8">
   	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
   	    <title>GCS browser: test-bucket</title>
		<style>
		header {
			margin-left: 10px;
		}

		.next-button {
			margin: 10px 0;
		}

		.grid-head {
			border-bottom: 1px solid black;
		}

		.resource-grid {
			margin-right: 20px;
		}

		li.grid-row:nth-child(even) {
			background-color: #ddd;
		}

		li div {
			box-sizing: border-box;
			border-left: 1px solid black;
			padding-left: 5px;
			overflow-wrap: break-word;
		}
		li div:first-child {
			border-left: none;
		}

		</style>
   	</head>
   	<body>

    <header>
        <h1>test-bucket</h1>
        <h3>/test-bucket/pr-logs/12345/</h3>
    </header>
    <ul class="resource-grid">

	<li class="pure-g">
		<div class="pure-u-2-5 grid-head">Name</div>
		<div class="pure-u-1-5 grid-head">Size</div>
		<div class="pure-u-2-5 grid-head">Modified</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/"><img src="/icons/back.png"> ..</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/1/"><img src="/icons/dir.png"> 1/</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/2/"><img src="/icons/dir.png"> 2/</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/file1"><img src="/icons/file.png"> file1</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 00:00:00 UTC</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/file2"><img src="/icons/file.png"> file2</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 22:00:00 UTC</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/file3"><img src="/icons/file.png"> file3</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 23:00:00 UTC</div>
	</li>
</ul>
<details>
	<summary style="display: list-item; padding-left: 1em">Download</summary>
	<div style="padding: 1em">
		You can download this directory by running the following <a href="https://cloud.google.com/storage/docs/gsutil">gsutil</a> command:
		<pre>gsutil -m cp -r gs://test-bucket/pr-logs/12345/ .</pre>
	</div>
</details>
</body></html>`,
		},
		{
			id:          "happy S3 case",
			path:        "/s3/test-bucket/pr-logs/12345/",
			initialDirs: []string{"/1", "/2"},
			initialObjects: []gcsObject{
				{
					BucketName: "s3://test-bucket",
					Name:       "/pr-logs/12345/file1",
					Updated:    time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "s3://test-bucket",
					Name:       "/pr-logs/12345/file2",
					Updated:    time.Date(2000, time.January, 1, 22, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "s3://test-bucket",
					Name:       "/pr-logs/12345/file3",
					Updated:    time.Date(2000, time.January, 1, 23, 0, 0, 0, time.UTC),
				},
			},
			expected: `
    <!doctype html>
   	<html>
   	<head>
   	    <link rel="stylesheet" type="text/css" href="/styles/style.css">
   	    <meta charset="utf-8">
   	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
   	    <title>S3 browser: test-bucket</title>
		<style>
		header {
			margin-left: 10px;
		}

		.next-button {
			margin: 10px 0;
		}

		.grid-head {
			border-bottom: 1px solid black;
		}

		.resource-grid {
			margin-right: 20px;
		}

		li.grid-row:nth-child(even) {
			background-color: #ddd;
		}

		li div {
			box-sizing: border-box;
			border-left: 1px solid black;
			padding-left: 5px;
			overflow-wrap: break-word;
		}
		li div:first-child {
			border-left: none;
		}

		</style>
   	</head>
   	<body>

    <header>
        <h1>test-bucket</h1>
        <h3>/test-bucket/pr-logs/12345/</h3>
    </header>
    <ul class="resource-grid">

	<li class="pure-g">
		<div class="pure-u-2-5 grid-head">Name</div>
		<div class="pure-u-1-5 grid-head">Size</div>
		<div class="pure-u-2-5 grid-head">Modified</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/"><img src="/icons/back.png"> ..</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/12345/1/"><img src="/icons/dir.png"> 1/</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/12345/2/"><img src="/icons/dir.png"> 2/</a></div>
	    <div class="pure-u-1-5">-</div>
	    <div class="pure-u-2-5">-</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/12345/file1"><img src="/icons/file.png"> file1</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 00:00:00 UTC</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/12345/file2"><img src="/icons/file.png"> file2</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 22:00:00 UTC</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/s3/test-bucket/pr-logs/12345/file3"><img src="/icons/file.png"> file3</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">Sat, 01 Jan 2000 23:00:00 UTC</div>
	</li>
</ul>
</body></html>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			w := httptest.NewRecorder()
			s := server{storageClient: newFakeOpener(tc.initialObjects, tc.initialDirs)}

			prowPath, err := parsePath(tc.path)
			if err != nil {
				t.Fatal(err)
			}

			if err := s.handleDirectory(w, prowPath, tc.path); err != nil {
				t.Fatalf("error not expected: %v", err)
			}

			actualBody := w.Body.String()
			if !reflect.DeepEqual(actualBody, tc.expected) {
				t.Fatal(cmp.Diff(actualBody, tc.expected))
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	testCases := []struct {
		id            string
		path          string
		expected      string
		errorExpected bool
	}{
		{
			id:       "GCS",
			path:     "/gcs/bucket/",
			expected: "gs://bucket",
		},
		{
			id:       "GCS without trailing slash",
			path:     "/gcs/bucket",
			expected: "gs://bucket",
		},
		{
			id:       "GCS with path",
			path:     "/gcs/bucket/path/to/object/",
			expected: "gs://bucket/path/to/object",
		},
		{
			id:       "S3",
			path:     "/s3/bucket/",
			expected: "s3://bucket",
		},
		{
			id:       "S3 with path",
			path:     "/s3/bucket/path/to/object/",
			expected: "s3://bucket/path/to/object",
		},
		{
			id:            "Only GCS prefix",
			path:          "/gcs/",
			errorExpected: true,
		},
		{
			id:            "Only GCS prefix without trailing slash",
			path:          "/gcs",
			errorExpected: true,
		},
		{
			id:            "Only S3 prefix",
			path:          "/s3/",
			errorExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			actual, err := parsePath(tc.path)
			if err != nil && !tc.errorExpected {
				t.Fatalf("Error not expected: %v", err)
			}
			if err == nil && tc.errorExpected {
				t.Fatalf("Error was expected")
			}

			if !tc.errorExpected {
				expected, err := prowv1.ParsePath(tc.expected)
				if err != nil {
					t.Fatal(err)
				}

				if !reflect.DeepEqual(expected, actual) {
					t.Fatal(cmp.Diff(expected, actual))
				}
			}
		})
	}
}

func TestPathPrefix(t *testing.T) {
	testCases := []struct {
		id       string
		prowPath string
		expected string
	}{
		{
			id:       "GCS",
			prowPath: "gs://bucket",
			expected: "/gcs/bucket",
		},
		{
			id:       "GCS without prefix",
			prowPath: "bucket",
			expected: "/gcs/bucket",
		},
		{
			id:       "S3",
			prowPath: "s3://bucket",
			expected: "/s3/bucket",
		},
		{
			id:       "With object path",
			prowPath: "gs://bucket/path/to/object",
			expected: "/gcs/bucket",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			prowPath, err := prowv1.ParsePath(tc.prowPath)
			if err != nil {
				t.Fatal(err)
			}

			actual := pathPrefix(prowPath)
			if !reflect.DeepEqual(tc.expected, actual) {
				t.Fatal(cmp.Diff(tc.expected, actual))
			}
		})
	}
}

func TestGetParent(t *testing.T) {
	testCases := []struct {
		id       string
		path     string
		expected string
	}{
		{
			id:       "happy case",
			path:     "/gcs/foo/bar",
			expected: "/gcs/foo/",
		},
		{
			id:       "trailing slash",
			path:     "/gcs/foo/bar/",
			expected: "/gcs/foo/",
		},
		{
			id:       "bucket root with trailing slash",
			path:     "/gcs/foo/",
			expected: "",
		},
		{
			id:       "bucket root without trailing slash",
			path:     "/gcs/foo",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			actual := getParent(tc.path)
			if !reflect.DeepEqual(tc.expected, actual) {
				t.Fatalf(cmp.Diff(tc.expected, actual))
			}
		})
	}
}
