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
	"net/url"
	"testing"
)

func Test_SetURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		err    bool
		bucket string
		object string
	}{
		{
			name:   "only bucket",
			url:    "gs://thisbucket",
			bucket: "thisbucket",
		},
		{
			name:   "bucket and object",
			url:    "gs://first/second",
			bucket: "first",
			object: "second",
		},
		{
			name: "reject files",
			url:  "/path/to/my/bucket",
			err:  true,
		},
		{
			name: "reject websites",
			url:  "http://example.com/object",
			err:  true,
		},
		{
			name: "reject ports",
			url:  "gs://first:123/second",
			err:  true,
		},
		{
			name: "reject username",
			url:  "gs://erick@first/second",
			err:  true,
		},
		{
			name: "reject queries",
			url:  "gs://first/second?query=true",
			err:  true,
		},
		{
			name: "reject fragments",
			url:  "gs://first/second#fragment",
			err:  true,
		},
	}
	for _, tc := range cases {
		var p Path
		err := p.Set(tc.url)
		switch {
		case err != nil && !tc.err:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case err == nil && tc.err:
			t.Errorf("%s: failed to raise an error", tc.name)
		default:
			if p.Bucket() != tc.bucket {
				t.Errorf("%s: bad bucket %s != %s", tc.name, p.Bucket(), tc.bucket)
			}
			if p.Object() != tc.object {
				t.Errorf("%s: bad object %s != %s", tc.name, p.Object(), tc.object)
			}
		}
	}
}

func Test_ResolveReference(t *testing.T) {
	var p Path
	err := p.Set("gs://bucket/path/to/config")
	if err != nil {
		t.Fatalf("bad path: %v", err)
	}
	u, err := url.Parse("testgroup")
	if err != nil {
		t.Fatalf("bad url: %v", err)
	}
	q, err := p.ResolveReference(u)
	if q.Object() != "path/to/testgroup" {
		t.Errorf("bad object: %s", q)
	}
	if q.Bucket() != "bucket" {
		t.Errorf("bad bucket: %s", q)
	}
}

// Ensure that a == b => calcCRC(a) == calcCRC(b)
func Test_calcCRC(t *testing.T) {
	b1 := []byte("hello")
	b2 := []byte("world")
	b1a := []byte("h")
	b1a = append(b1a, []byte("ello")...)
	c1 := calcCRC(b1)
	c2 := calcCRC(b2)
	c1a := calcCRC(b1a)

	switch {
	case c1 == c2:
		t.Errorf("g1 crc %d should not equal g2 crc %d", c1, c2)
	case len(b1) == 0, len(b2) == 0:
		t.Errorf("empty b1 b2 %s %s", b1, b2)
	case len(b1) != len(b1a), c1 != c1a:
		t.Errorf("different results: %s %d != %s %d", b1, c1, b1a, c1a)
	}

}
