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

// Package diskcache implements disk backed cache storage for use in greenhouse
package diskcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/test-infra/greenhouse/diskutil"

	"github.com/sirupsen/logrus"
)

// ReadHandler should be implemented by cache users for use with Cache.Get
type ReadHandler func(exists bool, contents io.ReadSeeker) error

// Cache implements disk backed cache storage
type Cache struct {
	diskRoot string
	logger   *logrus.Entry
}

// NewCache returns a new Cache given the root directory that should be used
// on disk for cache storage
func NewCache(diskRoot string) *Cache {
	return &Cache{
		diskRoot: strings.TrimSuffix(diskRoot, string(os.PathListSeparator)),
	}
}

// KeyToPath converts a cache entry key to a path on disk
func (c *Cache) KeyToPath(key string) string {
	return filepath.Join(c.diskRoot, key)
}

// PathToKey converts a path on disk to a key, assuming the path is actually
// under DiskRoot() ...
func (c *Cache) PathToKey(key string) string {
	return strings.TrimPrefix(key, c.diskRoot+string(os.PathSeparator))
}

// DiskRoot returns the root directory containing all on-disk cache entries
func (c *Cache) DiskRoot() string {
	return c.diskRoot
}

// file path helper
func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// file path helper
func ensureDir(dir string) error {
	if exists(dir) {
		return nil
	}
	return os.MkdirAll(dir, os.FileMode(0744))
}

func removeTemp(path string) {
	err := os.Remove(path)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to remove a temp file: %v", path)
	}
}

// Put copies the content reader until the end into the cache at key
// if contentSHA256 is not "" then the contents will only be stored in the
// cache if the content's hex string SHA256 matches
func (c *Cache) Put(key string, content io.Reader, contentSHA256 string) error {
	// make sure directory exists
	path := c.KeyToPath(key)
	dir := filepath.Dir(path)
	err := ensureDir(dir)
	if err != nil {
		logrus.WithError(err).Errorf("error ensuring directory '%s' exists", dir)
	}

	// create a temp file to get the content on disk
	temp, err := ioutil.TempFile(dir, "temp-put")
	if err != nil {
		return fmt.Errorf("failed to create cache entry: %v", err)
	}

	// fast path copying when not hashing content,s
	if contentSHA256 == "" {
		_, err = io.Copy(temp, content)
		if err != nil {
			removeTemp(temp.Name())
			return fmt.Errorf("failed to copy into cache entry: %v", err)
		}

	} else {
		hasher := sha256.New()
		_, err = io.Copy(io.MultiWriter(temp, hasher), content)
		if err != nil {
			removeTemp(temp.Name())
			return fmt.Errorf("failed to copy into cache entry: %v", err)
		}
		actualContentSHA256 := hex.EncodeToString(hasher.Sum(nil))
		if actualContentSHA256 != contentSHA256 {
			removeTemp(temp.Name())
			return fmt.Errorf(
				"hashes did not match for '%s', given: '%s' actual: '%s",
				key, contentSHA256, actualContentSHA256)
		}
	}

	// move the content to the key location
	err = temp.Sync()
	if err != nil {
		removeTemp(temp.Name())
		return fmt.Errorf("failed to sync cache entry: %v", err)
	}
	temp.Close()
	err = os.Rename(temp.Name(), path)
	if err != nil {
		removeTemp(temp.Name())
		return fmt.Errorf("failed to insert contents into cache: %v", err)
	}
	return nil
}

// Get provides your readHandler with the contents at key
func (c *Cache) Get(key string, readHandler ReadHandler) error {
	path := c.KeyToPath(key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return readHandler(false, nil)
		}
		return fmt.Errorf("failed to get key: %v", err)
	}
	return readHandler(true, f)
}

// EntryInfo are returned when getting entries from the cache
type EntryInfo struct {
	Path       string
	LastAccess time.Time
}

// GetEntries walks the cache dir and returns all paths that exist
// In the future this *may* be made smarter
func (c *Cache) GetEntries() []EntryInfo {
	entries := []EntryInfo{}
	// note we swallow errors because we just need to know what keys exist
	// some keys missing is OK since this is used for eviction, but not returning
	// any of the keys due to some error is NOT
	_ = filepath.Walk(c.diskRoot, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			logrus.WithError(err).Error("error getting some entries")
			return nil
		}
		if !f.IsDir() {
			atime := diskutil.GetATime(path, time.Now())
			entries = append(entries, EntryInfo{
				Path:       path,
				LastAccess: atime,
			})
		}
		return nil
	})
	return entries
}

// Delete deletes the file at key
func (c *Cache) Delete(key string) error {
	return os.Remove(c.KeyToPath(key))
}
