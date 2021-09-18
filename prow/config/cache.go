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
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cache"
	"k8s.io/test-infra/prow/git/v2"
)

// Overview
//
// Consider the expensive function prowYAMLGetter(), which needs to use a Git
// client, walk the filesystem path, etc. To speed things up, we save results of
// this function into a cache named ProwYAMLCache.

// ProwYAMLCache is the user-facing cache. It acts as a wrapper around the
// generic LRUCache, by handling type casting in and out of the LRUCache (which
// only handles empty interfaces).
type ProwYAMLCache cache.LRUCache

// NewProwYAMLCache creates a new LRU cache for ProwYAML values, where the keys
// are CacheKeys (that is, JSON strings) and values are pointers to ProwYAMLs.
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

// MakeCacheKey simply bundles up the given arguments into a CacheKeyParts
// struct, then converts it into a CacheKey (string).
func MakeCacheKey(identifier string, baseSHA string, headSHAs []string) (CacheKey, error) {
	kp := CacheKeyParts{
		Identifier: identifier,
		BaseSHA:    baseSHA,
		HeadSHAs:   headSHAs,
	}

	return kp.CacheKey()
}

// CacheKey converts a CacheKeyParts object into a JSON string (to be used as a
// CacheKey).
func (kp *CacheKeyParts) CacheKey() (CacheKey, error) {
	data, err := json.Marshal(kp)
	if err != nil {
		return "", err
	}

	return CacheKey(data), nil
}

// GetPresubmitsCached is like GetPresubmits, but uses a cache lookup to get the
// *ProwYAML value (cache hit), instead of computing it from scratch (cache
// miss). It also stores the *ProwYAML into the cache if there is a cache miss.
func (c *Config) GetPresubmitsCached(pc *ProwYAMLCache, gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {

	prowYAML, err := c.GetProwYAMLCached(pc, c.getProwYAML, gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return append(c.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

// GetPostsubmitsCached is like GetPostsubmits, but attempts to use a cache
// lookup to get the *ProwYAML value (cache hit), instead of computing it from
// scratch (cache miss). It also stores the *ProwYAML into the cache if there is
// a cache miss.
func (c *Config) GetPostsubmitsCached(pc *ProwYAMLCache, gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {

	prowYAML, err := c.GetProwYAMLCached(pc, c.getProwYAML, gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return append(c.GetPostsubmitsStatic(identifier), prowYAML.Postsubmits...), nil
}

// GetProwYAMLCached uses ProwYAMLCache to first try to perform a lookup of
// previously-calculated *ProwYAML objects. The 'valConstructorHelper' is used
// in two ways. First it is used by the caching mechanism to lazily generate the
// value only when it is required (otherwise, if all threads had to generate the
// value, it would defeat the purpose of the cache in the first place). Second,
// it makes it easier to test this function, because unit tests can just provide
// its own function for constructing a *ProwYAML object (instead of needing to
// create an actual Git repo, etc.).
func (c *Config) GetProwYAMLCached(
	prowYAMLCache *ProwYAMLCache,
	valConstructorHelper func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error),
	gc git.ClientFactory,
	identifier string,
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (*ProwYAML, error) {

	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}

	// Abort if the InRepoConfig is not enabled for this identifier (org/repo).
	// It's important that we short-circuit here __before__ calling GetOrAdd()
	// because we do NOT want to add an empty &ProwYAML{} value in the cache
	// (because not only is it useless, but adding a useless entry also may
	// result in evicting a useful entry if the underlying cache is full and an
	// older (useful) key is evicted).
	if !c.InRepoConfigEnabled(identifier) {
		return &ProwYAML{}, nil
	}

	baseSHA, headSHAs, err := GetAndCheckRefs(baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	key, err := MakeCacheKey(identifier, baseSHA, headSHAs)
	if err != nil {
		return nil, err
	}

	valConstructor := func() (interface{}, error) {
		return valConstructorHelper(gc, identifier, baseSHAGetter, headSHAGetters...)
	}

	return prowYAMLCache.GetOrAdd(key, valConstructor)
}

// GetOrAdd is a type assertion wrapper around the values retrieved from the
// inner LRUCache object (which only understands empty interfaces for both keys
// and values). Users are expected to add their own GetOrAdd method for their
// own cached value.
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
	// uses "interface{}" for the type of its items.
	err = fmt.Errorf("Programmer error: expected value type '*config.ProwYAML', got '%T'", val)
	logrus.Error(err)
	return nil, err
}
