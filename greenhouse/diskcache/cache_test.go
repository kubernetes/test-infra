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

package diskcache

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func hashBytes(b []byte) string {
	hasher := sha256.New()
	hasher.Write(b)
	return hex.EncodeToString(hasher.Sum(nil))
}

func makeRandomBytes(n int) (b []byte, err error) {
	b = make([]byte, n)
	_, err = rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// test cache.PathToKey and cache.KeyToPath
func TestPathToKeyKeyToPath(t *testing.T) {
	cache := NewCache("/some/dir")
	testCases := []struct {
		Key          string
		ExpectedPath string
	}{
		{
			Key:          "key",
			ExpectedPath: filepath.Join("/some/dir", "key"),
		},
		{
			Key:          "namespaced/key",
			ExpectedPath: filepath.Join("/some/dir", "namespaced/key"),
		},
		{
			Key:          "entry/nested/a/few/times",
			ExpectedPath: filepath.Join("/some/dir", "entry/nested/a/few/times"),
		},
		{
			Key:          "foobar/cas/asdf",
			ExpectedPath: filepath.Join("/some/dir", "foobar/cas/asdf"),
		},
		{
			Key:          "foobar/ac/asdf",
			ExpectedPath: filepath.Join("/some/dir", "foobar/ac/asdf"),
		},
	}
	for _, tc := range testCases {
		path := cache.KeyToPath(tc.Key)
		if path != tc.ExpectedPath {
			t.Fatalf("expected KeyToPath(%s) to be %s", tc.Key, path)
		}
		backToKey := cache.PathToKey(path)
		if backToKey != tc.Key {
			t.Fatalf("cache.KeyToPath(cache.PathToKey(%s)) was not idempotent, got %s", tc.Key, backToKey)
		}
	}
}

// test stateful cache methods
func TestCacheStorage(t *testing.T) {
	// create a cache in a tempdir
	dir, err := ioutil.TempDir("", "cache-tests")
	if err != nil {
		t.Fatalf("Failed to create tempdir for tests! %v", err)
	}
	defer os.RemoveAll(dir)
	cache := NewCache(dir)

	// sanity checks
	if cache.DiskRoot() != dir {
		t.Fatalf("Expected DiskRoot to be %v not %v", dir, cache.DiskRoot())
	}
	// we haven't put anything yet, so get should return exists == false
	err = cache.Get("some/key", func(exists bool, contents io.ReadSeeker) error {
		if exists {
			t.Fatal("no keys should exist yet!")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Got unexpected error testing non-existent key: %v", err)
	}

	// Test Put and Get together
	// create 1 MB of random bytes
	lotsOfRandomBytes, err := makeRandomBytes(1000000)
	if err != nil {
		t.Fatalf("Failed to create random test data: %v", err)
	}
	testCases := []struct {
		Name           string
		Key            string
		Contents       []byte
		Hash           string
		PutShouldError bool
	}{
		{
			Name:           "Normal",
			Key:            "foo",
			Contents:       []byte{1, 3, 3, 7},
			Hash:           hashBytes([]byte{1, 3, 3, 7}),
			PutShouldError: false,
		},
		{
			Name:           "Bad Hash",
			Key:            "test/foo/baz",
			Contents:       []byte{1, 3, 3, 7},
			Hash:           hashBytes([]byte{3, 1, 3, 3, 7}),
			PutShouldError: true,
		},
		{
			Name:           "Normal with Path Segments",
			Key:            "test/bar/baz",
			Contents:       []byte{107, 56, 115},
			Hash:           hashBytes([]byte{107, 56, 115}),
			PutShouldError: false,
		},
		{
			Name:           "Lots of Random Bytes",
			Key:            "a/b/c",
			Contents:       lotsOfRandomBytes,
			Hash:           hashBytes(lotsOfRandomBytes),
			PutShouldError: false,
		},
	}
	expectedKeys := sets.NewString()
	for _, tc := range testCases {
		err := cache.Put(tc.Key, bytes.NewReader(tc.Contents), tc.Hash)
		if err != nil && !tc.PutShouldError {
			t.Fatalf("Got error '%v' for test case '%s' and expected none.", err, tc.Name)
		} else if err == nil && tc.PutShouldError {
			t.Fatalf("Did not get error for test case '%s' and expected one.", tc.Name)
		} else if err == nil {
			expectedKeys.Insert(tc.Key)
		}

		err = cache.Get(tc.Key, func(exists bool, contents io.ReadSeeker) error {
			if exists && tc.PutShouldError {
				t.Fatalf("Got key exists for test case '%s' which should not.", tc.Name)
			} else if !exists && !tc.PutShouldError {
				t.Fatalf("Got key does not exist for test case '%s' which should.", tc.Name)
			}
			if exists {
				read, err2 := ioutil.ReadAll(contents)
				if err2 != nil {
					t.Fatalf("Failed to read contents for test case '%s", tc.Name)
				}
				if !bytes.Equal(read, tc.Contents) {
					t.Fatalf("Contents did not match expected for test case '%s' (got: %v expected: %v)", tc.Name, read, tc.Contents)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Got unepected error getting cache key for test case '%s': %v", tc.Name, err)
		}
	}

	// test GetEntries
	entries := cache.GetEntries()
	receivedKeys := sets.NewString()
	for _, entry := range entries {
		receivedKeys.Insert(cache.PathToKey(entry.Path))
	}
	if !expectedKeys.Equal(receivedKeys) {
		t.Fatalf("entries %v does not equal expected: %v", receivedKeys, expectedKeys)
	}

	// test deleting all keys
	for _, key := range expectedKeys.List() {
		err = cache.Delete(key)
		if err != nil {
			t.Fatalf("failed to delete key: %v", err)
		}
	}
	entries = cache.GetEntries()
	if len(entries) != 0 {
		t.Fatalf("cache.GetEntries() should be empty after deleting all keys, got: %v", entries)
	}
}
