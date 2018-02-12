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

// cache implements disk backed cache storage for use in nursery
package diskcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// ReadHandler should be implemeted by cache users for use with Cache.Get
type ReadHandler func(exists bool, contents io.ReadSeeker) error

// Cache implements disk backed cache storage
type Cache struct {
	diskRoot string
}

// NewCache returns a new Cache given the root directory that should be used
// on disk for cache storage
func NewCache(diskRoot string) *Cache {
	return &Cache{
		diskRoot,
	}
}

func (c *Cache) keyToPath(key string) string {
	return filepath.Join(c.diskRoot, key)
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
		log.WithError(err).Errorf("Failed to remove a temp file: %v", path)
	}
}

// Put copies the content reader until the end into the cache at key
// if contentSHA256 is not "" then the contents will only be stored in the
// cache if the content's hex string SHA256 matches
func (c *Cache) Put(key string, content io.Reader, contentSHA256 string) error {
	// make sure directory exists
	path := c.keyToPath(key)
	dir := filepath.Dir(path)
	err := ensureDir(dir)
	if err != nil {
		log.WithError(err).Errorf("error ensuring directory '%s' exists", dir)
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
	path := c.keyToPath(key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return readHandler(false, nil)
		}
		return fmt.Errorf("failed to get key: %v", err)
	}
	return readHandler(true, f)
}
