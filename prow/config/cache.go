/*
Copyright 2021 The Kubernetes Authors.

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

package config

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cache"
	"k8s.io/test-infra/prow/git/v2"
)

// Overview
//
// Consider the expensive function prowYAMLGetter(). This function is
// expensive to compute (it involves invoking a Git client, walking a filesystem
// path, etc). In our caching implementation, we save results of this function
// into a cache (named ProwYAMLCache). The point is to avoid doing the expensive
// GetPresubmits() and GetPostsubmits() calls if possible, which involves
// walking the repository to collect YAML information, etc.

// ProwYAMLCache is the user-facing cache. It acts as a wrapper around the
// generic LRUCache, by handling type casting in and out of the LRUCache (which
// only handles empty interfaces).
type ProwYAMLCache cache.LRUCache

// NewProwYAMLCache creates a new LRU cache for ProwYAML values, where the keys
// are CacheKeys and values are pointers to ProwYAMLs.
func NewProwYAMLCache(size int) (*ProwYAMLCache, error) {
	cache, err := cache.NewLRUCache(size)
	if err != nil {
		return nil, err
	}

	return (*ProwYAMLCache)(cache), nil
}

// CacheKey acts as a key to the ProwYAMLCache. We construct it by marshaling
// CacheKeyParts into a JSON string.
type CacheKey string

// The CacheKeyParts is a struct because we want to keep the various components
// that make up the key separate to help keep tests readable. Because the
// headSHAs field is a slice, the overall CacheKey object is not hashable and
// cannot be used directly as a key. Instead we marshal it to JSON first, then
// convert its type to CacheKey.
//
// Users should take care to ensure that headSHAs remains stable (order
// matters).
type CacheKeyParts struct {
	Identifier string   `json:"identifier"`
	BaseSHA    string   `json:"baseSHA"`
	HeadSHAs   []string `json:"headSHAs"`
}

// MakeCacheKeyParts constructs a CacheKeyParts struct from uniquely-identifying
// information.
func MakeCacheKeyParts(
	identifier string,
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (CacheKeyParts, error) {
	// Initialize empty key parts.
	keyParts := CacheKeyParts{}

	// Append "identifier" string information.
	if identifier == "" {
		return CacheKeyParts{}, fmt.Errorf("identifier cannot be empty")
	}
	keyParts.Identifier = identifier

	// Append "baseSHA" string information.
	baseSHA, err := baseSHAGetter()
	if err != nil {
		return CacheKeyParts{}, fmt.Errorf("failed to get baseSHA: %v", err)
	}
	keyParts.BaseSHA = baseSHA

	// Append "headSHAs" string information.
	var headSHAs []string
	for _, headSHAGetter := range headSHAGetters {
		headSHA, err := headSHAGetter()
		if err != nil {
			return CacheKeyParts{}, fmt.Errorf("failed to get headRef: %v", err)
		}
		headSHAs = append(headSHAs, headSHA)
	}

	keyParts.HeadSHAs = headSHAs

	return keyParts, nil
}

// MakeCacheKey converts a CacheKeyParts object into a JSON string (to be used
// as a CacheKey).
func MakeCacheKey(kp CacheKeyParts) (CacheKey, error) {
	data, err := json.Marshal(kp)

	return CacheKey(data), err
}

// GetPresubmitsFromCache is like GetPresubmits, but attempts to use a cache
// lookup to get the *ProwYAML value (cache hit), instead of computing it from
// scratch (cache miss).
func (c *Config) GetPresubmitsFromCache(pc *ProwYAMLCache, gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(pc, c.getProwYAMLNoDefault, gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return append(c.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

// GetPostsubmitsFromCache is like GetPostsubmits, but attempts to use a cache
// lookup to get the *ProwYAML value (cache hit), instead of computing it from
// scratch (cache miss).
func (c *Config) GetPostsubmitsFromCache(pc *ProwYAMLCache, gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(pc, c.getProwYAMLNoDefault, gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return append(c.GetPostsubmitsStatic(identifier), prowYAML.Postsubmits...), nil
}

// GetProwYAMLFromCache uses ProwYAMLCache to first try to perform a lookup of
// previously-calculated ProwYAML objects. The 'valConstructor' function is
// taken as an argument to make it easier to test this function. This way, unit
// tests can just provide its own function for constructing a ProwYAML object
// (instead of needing to create an actual Git repo, etc.).
func GetProwYAMLFromCache(
	prowYAMLCache *ProwYAMLCache,
	valConstructorHelper func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error),
	gc git.ClientFactory,
	identifier string,
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (*ProwYAML, error) {

	kp, err := MakeCacheKeyParts(identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	key, err := MakeCacheKey(kp)
	if err != nil {
		return nil, err
	}

	// The point of valConstructor is to allow us to mock this expensive value
	// constructor call in tests (and, e.g., avoid having to do things like
	// going over the network or dealing with an actual Git client, etc.).
	valConstructor := func() (interface{}, error) {
		return valConstructorHelper(gc, identifier, baseSHAGetter, headSHAGetters...)
	}

	return prowYAMLCache.GetOrAdd(key, valConstructor)
}

// GetOrAdd is a type casting wrapper around the inner LRUCache object. Users
// are expected to add their own GetOrAdd method for their own cached type.
func (p *ProwYAMLCache) GetOrAdd(
	key CacheKey,
	valConstructor cache.ValConstructor) (*ProwYAML, error) {

	val, err := (*cache.LRUCache)(p).GetOrAdd(key, valConstructor)
	if err != nil {
		return nil, err
	}

	prowYAML, ok := val.(*ProwYAML)
	if ok {
		return prowYAML, err
	}

	// Somehow, the value retrieved with GetOrAdd has the wrong type. This can
	// happen if some other function modified the cache and put in the wrong
	// type. Ultimately, this is a price we pay for using a cache library that
	// uses "interface{}" for the type of its items. In this case, we log an
	// error message and return an error.
	err = fmt.Errorf("Programmer error: expected value type '*config.ProwYAML', got '%T'", val)
	logrus.Error(err)
	return nil, err
}
