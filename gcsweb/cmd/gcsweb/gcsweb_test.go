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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	prowv1 "sigs.k8s.io/prow/prow/apis/prowjobs/v1"
	pkgio "sigs.k8s.io/prow/prow/io"
	"sigs.k8s.io/prow/prow/io/fakeopener"
)

type fakeOpener struct {
	fakeopener.FakeOpener

	objects     []gcsObject
	directories []string
}

func mustParseProwPath(bucket string) *prowv1.ProwPath {
	path, err := prowv1.ParsePath(bucket)
	if err != nil {
		panic(fmt.Sprintf("cannot parse prow path %s", bucket))
	}
	return path
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

func TestParseBucket(t *testing.T) {
	aliasPath := func(bucket string) string { return pathPrefix(mustParseProwPath(bucket)) }

	for _, testCase := range []struct {
		name        string
		bucket      string
		wantErr     error
		wantOptions *options
	}{
		{
			name:   "Parse a bucket name with no alias",
			bucket: "test-infra-bucket",
			wantOptions: &options{
				allowedProwPaths: []*prowv1.ProwPath{mustParseProwPath("test-infra-bucket")},
			},
		},
		{
			name:   "Parse a bucket name with an alias",
			bucket: "test-infra-bucket=gs://test-infra-alias-1",
			wantOptions: &options{
				allowedProwPaths: []*prowv1.ProwPath{
					mustParseProwPath("test-infra-bucket"),
					mustParseProwPath("test-infra-alias-1"),
				},
				bucketAliases: bucketAliases{
					aliasPath("test-infra-alias-1"): aliasPath("test-infra-bucket"),
				},
			},
		},
		{
			name:   "Parse a bucket name with multiple aliases",
			bucket: "test-infra-bucket=gs://test-infra-alias-1,test-infra-alias-2",
			wantOptions: &options{
				allowedProwPaths: []*prowv1.ProwPath{
					mustParseProwPath("test-infra-bucket"),
					mustParseProwPath("gs://test-infra-alias-1"),
					mustParseProwPath("test-infra-alias-2"),
				},
				bucketAliases: bucketAliases{
					aliasPath("test-infra-alias-1"):      aliasPath("test-infra-bucket"),
					aliasPath("gs://test-infra-alias-1"): aliasPath("test-infra-bucket"),
					aliasPath("test-infra-alias-2"):      aliasPath("test-infra-bucket"),
				},
			},
		},
		{
			name:   "Deduplicate aliases",
			bucket: "test-infra-bucket=gs://test-infra-alias-1,test-infra-alias-1",
			wantOptions: &options{
				allowedProwPaths: []*prowv1.ProwPath{
					mustParseProwPath("test-infra-bucket"),
					mustParseProwPath("gs://test-infra-alias-1"),
				},
				bucketAliases: bucketAliases{
					aliasPath("test-infra-alias-1"):      aliasPath("test-infra-bucket"),
					aliasPath("gs://test-infra-alias-1"): aliasPath("test-infra-bucket"),
				},
			},
		},
		{
			name:    "Fail to parse: aliases expected",
			bucket:  "test-infra-bucket=",
			wantErr: errors.New(`empty alias for bucket "test-infra-bucket" is not a allowed`),
		},
		{
			name:    "Fail to parse: bucket name is empty",
			bucket:  "",
			wantErr: errors.New("empty bucket name is not allowed"),
		},
		{
			name:    "Fail to parse: invalid bucket name",
			bucket:  string([]byte{0x0}),
			wantErr: errors.New(`bucket "\x00" is not a valid bucket: path "gs://\x00" has invalid format, expected either <bucket-name>[/<path>] or <storage-provider>://<bucket-name>[/<path>]`),
		},
		{
			name:    "Fail to parse: invalid alias name",
			bucket:  fmt.Sprintf("test-infra-bucket:=%s", string([]byte{0x0})),
			wantErr: errors.New(`bucket alias "\x00" is not a valid bucket: path "gs://\x00" has invalid format, expected either <bucket-name>[/<path>] or <storage-provider>://<bucket-name>[/<path>]`),
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.wantOptions != nil && testCase.wantOptions.bucketAliases == nil {
				testCase.wantOptions.bucketAliases = bucketAliases{}
			}
			o := &options{bucketAliases: bucketAliases{}}
			_, err := o.parseBucket(testCase.bucket)

			if err != nil && testCase.wantErr == nil {
				t.Fatalf("want err nil but got: %v", err)
			}
			if err == nil && testCase.wantErr != nil {
				t.Fatalf("want err %v but got nil", testCase.wantErr)
			}
			if err != nil && testCase.wantErr != nil {
				if diff := cmp.Diff(testCase.wantErr.Error(), err.Error()); diff != "" {
					t.Fatalf("unexpected error: %s", diff)
				}
				return
			}

			if diff := cmp.Diff(testCase.wantOptions.bucketAliases, o.bucketAliases); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(testCase.wantOptions.allowedProwPaths, o.allowedProwPaths); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestBucketAlias(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		path     string
		aliases  bucketAliases
		wantPath string
	}{
		{
			name:     "Do not match",
			path:     "/bar",
			aliases:  bucketAliases{"/foo": ""},
			wantPath: "/bar",
		},
		{
			name:     "Match and rewrite",
			path:     "/foo/bar/baz",
			aliases:  bucketAliases{"/foo/bar": "/super"},
			wantPath: "/super/baz",
		},
		{
			name:     "Match and rewrite but path stays the same",
			path:     "/foo/bar/baz",
			aliases:  bucketAliases{"/foo/bar": "/foo/bar"},
			wantPath: "/foo/bar/baz",
		},
		{
			name:     "Remove prefix",
			path:     "/foo/bar/baz",
			aliases:  bucketAliases{"/foo/bar": ""},
			wantPath: "/baz",
		},
		{
			name:     "Add prefix",
			path:     "/foo/bar/baz",
			aliases:  bucketAliases{"": "/super/super"},
			wantPath: "/super/super/foo/bar/baz",
		},
		{
			name:     "Rewrite path once",
			path:     "/foo/foo/bar",
			aliases:  bucketAliases{"/foo": "/baz"},
			wantPath: "/baz/foo/bar",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			path := testCase.aliases.rewritePath(testCase.path)
			if path != testCase.wantPath {
				t.Fatalf("want path %q but got %q", testCase.wantPath, path)
			}
		})
	}
}
