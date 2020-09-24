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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/mux"

	"cloud.google.com/go/storage"

	"google.golang.org/api/option"
)

type gcsMockServer struct {
	ts          *httptest.Server
	objects     []gcsObject
	directories []string
}

func (s *gcsMockServer) initializeAndStartGCSMockServer(objects []gcsObject, dirs []string) {
	s.objects = objects
	s.directories = dirs

	m := mux.NewRouter()

	// Request all objects from a bucket
	m.Path("/b/{bucketName}/o").Methods("GET").HandlerFunc(s.listObjects)

	// Request a specific object
	m.Path("/b/{bucketName}/o/{objectName:.+}").Methods("GET").HandlerFunc(s.getObject)

	// This path represents the request of a raw file
	m.Host("{host:.+}").Path("/{path:.+}").Methods("GET").HandlerFunc(s.getObjectRaw)

	s.ts = httptest.NewUnstartedServer(m)
	s.ts.StartTLS()
}

type gcsObject struct {
	BucketName string    `json:"-"`
	Name       string    `json:"name"`
	Content    []byte    `json:"-"`
	Updated    time.Time `json:"updated,omitempty"`
}

type objectResponse struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	ID      string `json:"id"`
	Bucket  string `json:"bucket"`
	Size    int64  `json:"size,string"`
	Updated string `json:"updated,omitempty"`
}

func getObjectResponse(obj gcsObject) objectResponse {
	return objectResponse{
		Kind:    "storage#object",
		ID:      obj.BucketName + "/" + obj.Name,
		Bucket:  obj.BucketName,
		Name:    obj.Name,
		Size:    int64(len(obj.Content)),
		Updated: obj.Updated.Format("2006-01-02T15:04:05.999999Z07:00"),
	}
}

func (s *gcsMockServer) getObjectRaw(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]

	for _, obj := range s.objects {
		bucketName, objectName := splitBucketObject(path)

		if bucketName == obj.BucketName && objectName == obj.Name {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(obj.Content)))
			w.Header().Set("Last-Modified", obj.Updated.Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, string(obj.Content))
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *gcsMockServer) getObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bucketName := vars["bucketName"]
	objectName := vars["objectName"]
	encoder := json.NewEncoder(w)
	w.Header().Set("Accept-Ranges", "bytes")
	for _, obj := range s.objects {
		if bucketName == obj.BucketName && objectName == obj.Name {
			encoder.Encode(getObjectResponse(obj))
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

type objectListResponse struct {
	Kind     string        `json:"kind"`
	Items    []interface{} `json:"items"`
	Prefixes []string      `json:"prefixes,omitempty"`
}

func (s *gcsMockServer) listObjects(w http.ResponseWriter, r *http.Request) {
	encoder := json.NewEncoder(w)

	resp := objectListResponse{
		Items:    make([]interface{}, len(s.objects)),
		Prefixes: s.directories,
	}

	for i, obj := range s.objects {
		resp.Items[i] = getObjectResponse(obj)
	}

	encoder.Encode(resp)
}

func TestHandleObject(t *testing.T) {
	testCases := []struct {
		id              string
		initialObjects  []gcsObject
		bucket          string
		object          string
		path            string
		headers         objectHeaders
		expected        string
		expectedHeaders http.Header
		errorExpected   bool
	}{
		{
			id:     "happy case",
			bucket: "test-bucket",
			object: "path/to/file1",
			initialObjects: []gcsObject{
				{
					BucketName: "test-bucket",
					Name:       "path/to/file1",
					Content:    []byte("123456789"),
				},
				{
					BucketName: "test-bucket",
					Name:       "path/to/file2",
					Content:    []byte("0000000000"),
				},
			},
			expectedHeaders: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			expected:        "123456789",
		},
		{
			id:     "sad case",
			bucket: "test-bucket",
			object: "path/to/unknown",
			initialObjects: []gcsObject{
				{
					BucketName: "test-bucket",
					Name:       "path/to/file1",
					Content:    []byte("123456789"),
				},
				{
					BucketName: "test-bucket",
					Name:       "path/to/file2",
					Content:    []byte("0000000000"),
				},
			},
			expectedHeaders: http.Header{},
			errorExpected:   true,
		},
		{
			id:     "happy html case",
			bucket: "test-bucket",
			object: "path/to/file1",
			initialObjects: []gcsObject{
				{
					BucketName: "test-bucket",
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
				{
					BucketName: "test-bucket",
					Name:       "path/to/file2",
					Content:    []byte("0000000000"),
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

			mock := &gcsMockServer{}
			mock.initializeAndStartGCSMockServer(tc.initialObjects, []string{"/test-dir"})
			w := httptest.NewRecorder()

			httpClient := &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}}

			client, err := storage.NewClient(context.Background(), option.WithEndpoint(mock.ts.URL), option.WithHTTPClient(httpClient))
			if err != nil {
				t.Fatalf("couldn't create storage client: %v", err)
			}

			s := server{storageClient: client}

			err = s.handleObject(w, tc.bucket, tc.object, tc.headers)
			if err != nil && !tc.errorExpected {
				t.Fatalf("Error not expected: %v", err)
			}

			if err == nil && tc.errorExpected {
				t.Fatalf("Error was expected")
			}
			actualHeaders := w.HeaderMap
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
		bucket         string
		object         string
		path           string
		expected       string
	}{
		{
			id:          "happy case",
			bucket:      "test-bucket",
			object:      "pr-logs/12345",
			path:        "/test-bucket/pr-logs/12345/",
			initialDirs: []string{"/1", "/2"},
			initialObjects: []gcsObject{
				{
					BucketName: "test-bucket",
					Name:       "/pr-logs/12345/file1",
					Updated:    time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "test-bucket",
					Name:       "/pr-logs/12345/file2",
					Updated:    time.Date(2000, time.January, 1, 22, 0, 0, 0, time.UTC),
				},
				{
					BucketName: "test-bucket",
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
	    <div class="pure-u-2-5">01 Jan 2000 00:00:00</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/file2"><img src="/icons/file.png"> file2</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">01 Jan 2000 22:00:00</div>
	</li>

    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="/gcs/test-bucket/pr-logs/12345/file3"><img src="/icons/file.png"> file3</a></div>
	    <div class="pure-u-1-5">0</div>
	    <div class="pure-u-2-5">01 Jan 2000 23:00:00</div>
	</li>
</ul></body></html>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			mock := &gcsMockServer{}
			mock.initializeAndStartGCSMockServer(tc.initialObjects, tc.initialDirs)

			httpClient := &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}}

			client, err := storage.NewClient(context.TODO(), option.WithEndpoint(mock.ts.URL), option.WithHTTPClient(httpClient))
			if err != nil {
				t.Fatalf("couldn't create storage client: %v", err)
			}

			s := server{storageClient: client}
			w := httptest.NewRecorder()

			if err := s.handleDirectory(w, tc.bucket, tc.object, tc.path); err != nil {
				t.Fatalf("error not expected: %v", err)
			}

			actualBody := fmt.Sprintf("%s", w.Body.String())
			if !reflect.DeepEqual(actualBody, tc.expected) {
				t.Fatal(cmp.Diff(actualBody, tc.expected))
			}
		})
	}
}

func TestSplitBucketObject(t *testing.T) {
	testCases := []struct {
		id       string
		path     string
		expected []string
	}{
		{
			id:       "happy case",
			path:     "/bucket/path/to/object",
			expected: []string{"bucket", "path/to/object"},
		},
	}

	for _, tc := range testCases {
		bucket, object := splitBucketObject(tc.path)
		actual := []string{bucket, object}
		if !reflect.DeepEqual(tc.expected, actual) {
			t.Fatalf(cmp.Diff(tc.expected, actual))
		}
	}
}

func TestDirname(t *testing.T) {
	testCases := []struct {
		id       string
		path     string
		expected string
	}{
		{
			id:       "happy case",
			path:     "foo/bar",
			expected: "foo/",
		},
	}

	for _, tc := range testCases {
		dir := dirname(tc.path)
		if !reflect.DeepEqual(tc.expected, dir) {
			t.Fatalf(cmp.Diff(tc.expected, dir))
		}
	}
}
