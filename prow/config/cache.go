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
	"sort"
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/v2"
)

// Caching implementation overview
//
// Consider the expensive function GetPresubmits(). This function returns
// a []Presubmit slice, which is expensive to compute (it involves
// invoking a Git client, walking a filesystem path, etc). In our caching
// implementation, we call a wrapper function instead, GetPresubmitsFromCache().
// GetPresubmitsFromCache() can wrap around GetPresubmits(), and __only__
// calls it when the corresponding []Presubmit slice we want is not found
// in the cache (or if the cache is not initialized properly, or even
// corrupted).
//
// The same thing can be said for GetPostsubmits() and its wrapper,
// GetPostsubmitsFromCache() which we define in this file.
//
// The key idea is this: when we need to look up a value for the cache, we first
// look it up by its key. The function that creates a key ("keyConstructor"
// type) is fast and easy to compute. If we find an entry in the cache that
// matches the key that we just constructed, then return that. Otherwise if the
// value is not found, we (begrudgingly) compute it from scratch using the given
// "value constructor" function (aka the "valConstructor" type). For
// []Presubmit the constructor function is GetPresubmits(), and
// for []Postsubmit it is GetPostsubmits().
//
// The functions GetPresubmitsFromCache() and GetPostsubmitsFromCache() both
// implmement this key idea, with some additional protections around type safety
// guarantees, cache readiness (uninitialized cache), and cache corruption
// (where the value in the cache does not match what we want). We have to have
// two functions like this, one for each type, because Go does not yet support
// generics (to be added in Go 1.18). Lastly, these functions make use of a
// GetFromCache() helper function, so that we can test that separately (and also
// add unit tests that add presumptions about the underlying cache
// implementation's behavior).

// ProwYAMLCache holds Presubmits and Postsubmits in a simple cache. It is named
// ProwYAMLCache because the objects it caches resemble the fields that make up
// the ProwYAML object defined in inrepoconfig.go (i.e., the values of the cache
// are the []Presubmit and []Postsubmit objects). The point is to
// avoid doing the expensive GetPresubmits() and GetPostsubmits()
// calls if possible, which involves walking the repository to collect YAML
// information, etc.
//
// We use an off-the-shelf LRU cache library for the low-level caching
// implementation, which uses the empty interface for keys and values. The
// values are what we store in the cache, and to retrieve them, we have to
// provide a key (which must be a hashable object). Because of Golang's lack of
// generics, the values retrieved from the cache must be type-asserted into the
// type we want ([]Presubmit or []Postsubmit) before we can use
// them.
type ProwYAMLCache struct {
	LRUCache
}

type LRUCache struct {
	sync.Mutex
	*simplelru.LRU
}

type Promise struct {
	Result
	resolve chan struct{}
}

type Result struct {
	val interface{}
	err error
}

func NewLRUCache(size int) (*LRUCache, error) {
	cache, err := simplelru.NewLRU(size, nil)
	if err != nil {
		return nil, err
	}

	return &LRUCache{sync.Mutex{}, cache}, nil
}

// NewProwYAMLCache creates a new LRU cache for presubmits and postsubmits,
// where the keys are CacheKeys and values are ProwYAMLs.
func NewProwYAMLCache(size int) (*ProwYAMLCache, error) {
	cache, err := NewLRUCache(size)
	if err != nil {
		return nil, err
	}

	return &ProwYAMLCache{*cache}, nil
}

// InitProwYAMLCache calls NewProwYAMLCache() to initialize a cache of ProwYAMLs
// in the Config object. The cache sits inside the Config object because the
// function signature of GetPresubmitsFromCache() matches that of
// GetPresubmits() (which does not use the cache). The same reasoning applies
// for GetPostsubmitsFromCache() and GetPostsubmits(). This way, callers can use
// an interface to get presubmits in a cache-agnostic way.
func (c *Config) InitProwYAMLCache(size int) error {
	cache, err := NewProwYAMLCache(size)
	if err != nil {
		return err
	}

	c.ProwYAMLCache = cache
	return nil
}

// CacheKey acts as a key to either the ProwYAMLCache.presubmits or
// ProwYAMLCache.postsubmits cache. The CacheKey is a struct because we want to
// keep the various components that make up the key separate to help keep tests
// readable. Because the headSHAs field is a slice, the overall CacheKey object
// is not hashable and cannot be used directly as a key. Instead we use the
// fmt.Stringer interface implementation for it.
//
// Users should take care to ensure that headSHAs remains stable (order
// matters).
type CacheKeyParts struct {
	Identifier string   `json:"identifier"`
	BaseSHA    string   `json:"baseSHA"`
	HeadSHAs   []string `json:"headSHAs"`
}

type CacheKey string

// MakeCacheKeyParts constructs a CacheKeyParts struct from uniquely-identifying
// information. The only requirement is that we take all of the stringlike
// parameters and concatenate them together to form a UUID.
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

	// For determinism, sort the headSHAs, in case the caller has not sorted
	// them already.
	sort.Strings(headSHAs)
	keyParts.HeadSHAs = headSHAs

	return keyParts, nil
}

func MakeCacheKey(kp CacheKeyParts) (CacheKey, error) {
	// Convert to JSON string. This is a bit "heavy" but as long as we get a
	// deterministic string it doesn't matter.
	data, err := json.Marshal(kp)

	return CacheKey(data), err
}

// valConstructor is used to construct a value. The raw values of a cache are
// only constructed after a cache miss or as a general fallback to bypass the
// cache if the cache is unusable for whatever reason.
type valConstructor func() (interface{}, error)

type valConstructorHelper func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error)

// keyConstructor is used only when we need to perform a lookup inside a cache
// (if it is available), because all values stored in the cache are paired with
// a unique lookup key.
type keyConstructor func() (CacheKey, error)

// GetPresubmitsFromCache is like GetPresubmits, but attempts to use a cache
// lookup to get the prowYAML value (cache hit), instead of computing it from
// scratch (cache miss).
func (c *Config) GetPresubmitsFromCache(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(c.ProwYAMLCache, c.getProwYAML, gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return append(c.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

// GetPostsubmitsFromCache is like GetPostsubmits, but attempts to use a cache
// lookup to get the prowYAML value (cache hit), instead of computing it from
// scratch (cache miss).
func (c *Config) GetPostsubmitsFromCache(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(c.ProwYAMLCache, c.getProwYAML, gc, identifier, baseSHAGetter, headSHAGetters...)
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
	valConstructorHelper valConstructorHelper,
	gc git.ClientFactory,
	identifier string,
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (*ProwYAML, error) {

	keyConstructor := func() (CacheKey, error) {
		kp, err := MakeCacheKeyParts(identifier, baseSHAGetter, headSHAGetters...)
		if err != nil {
			return CacheKey(""), err
		}

		return MakeCacheKey(kp)
	}

	// The point of valConstructor is to allow us to mock this expensive value
	// constructor call in tests (and, e.g., avoid having to do things like
	// going over the network or dealing with an actual Git client, etc.).
	valConstructor := func() (interface{}, error) {
		return valConstructorHelper(gc, identifier, baseSHAGetter, headSHAGetters...)
	}

	val, err := prowYAMLCache.GetOrAdd(keyConstructor, valConstructor)
	if err != nil {
		return nil, err
	}

	prowYAML, ok := val.(*ProwYAML)
	if ok {
		return prowYAML, err
	}

	// Somehow, the value retrieved with GetFromCache has a malformed type. This
	// can happen if some other function modified the cache. Ultimately, this is
	// a price we pay for using a cache library that uses "interface{}" for the
	// type of its items. In this case, we log a warning and return an error.
	err = fmt.Errorf("cache value type error: expected value type '*config.ProwYAML', got '%T'", val)
	logrus.Warn(err)
	return nil, err
}

// GetOrAdd tries to use a cache if it is available to get a Value. It is
// assumed that Value is expensive to construct from scratch, which is the
// reason why we try to use the cache in the first place. If we do end up
// constructing a Value from scratch, then we store it into the cache with a
// corresponding Key, so that we can look up the Value with just the Key in the
// future.
//
// This code for a concurrent non-blocking cache with support for duplicate
// suppression is drawn from Alan Donovan and Brian Kernighan, "The Go
// Programming Language" (Addison-Wesley, 2016), p. 277.
func (lruCache *LRUCache) GetOrAdd(
	keyConstructor keyConstructor,
	valConstructor valConstructor) (interface{}, error) {

	// If the cache is unreachable, then fall back to cache-less behavior
	// (construct the value from scratch).
	if lruCache == nil {
		valConstructed, err := valConstructor()
		if err != nil {
			return nil, err
		}

		return valConstructed, nil
	}

	// Construct cache key. We use this key to find the value (if it was already
	// stored in the cache by a previous call to GetOrAdd).
	key, err := keyConstructor()
	if err != nil {
		return nil, err
	}

	// Cache lookup.
	lruCache.Lock()
	var promise *Promise
	var ok bool
	maybePromise, promisePending := lruCache.Get(key)

	if promisePending {
		// A promise exists, BUT the wrapped value inside it (p.result) might
		// not be written to yet by the thread that is actually resolving the
		// promise.
		//
		// For now we just unlock the overall lruCache itself so that it can
		// service other GetOrAdd() calls to it.
		lruCache.Unlock()

		// If the type is not a promise type, there's no need to wait and we can
		// just return immediately with an error.
		promise, ok = maybePromise.(*Promise)
		if !ok {
			return nil, fmt.Errorf("invalid cache entry type '%T', expected '*Promise'", maybePromise)
		}

		// Block until the first thread originally created this promise has
		// finished resolving it. Then it's safe to return the resolved values
		// of the promise below.
		//
		// If the original thread resolved the promise already a long time ago
		// (by closing the "resolve" channel), then this receive instruction
		// will finish immediately and we will not block at all.
		<-promise.resolve
	} else {
		// No promise exists for this key. In other words, we are the first
		// thread to ask for this key's value and so We have no choice but to
		// construct the value ourselves (this call is expensive!) and add it to
		// the cache.
		//
		// If there are other concurrent threads that call GetOrAdd() with the
		// same key and value constructors, we force them to use the same value
		// as us (so that they don't have to also all valConstructor()). We do
		// this with the following algorithm:
		//
		//  1. immediately create a promise to construct the value
		//  2. actually construct the value (expensive operation)
		//  3. resolve the promise to alert all threads looking at the same promise
		//     get the value from step 2.
		//
		// This mitigation strategy is a kind of "duplicate suppression", also
		// called "request coalescing". The problem of multiple requests for the
		// same cache entry is also called "cache stampede".

		// Step 1
		//
		// Let other threads know about our promise to construct the value. We
		// don't care if the underlying LRU cache had to evict an existing
		// entry.
		promise = &Promise{resolve: make(chan struct{})}
		_ = lruCache.Add(key, promise)
		lruCache.Unlock()

		// Step 2
		//
		// Construct the value (expensive operation).
		promise.val, promise.err = valConstructor()

		// Step 3
		//
		// Broadcast to all watchers of this promise that it is ready to be read
		// from (no data race!).
		close(promise.resolve)

		// If the value construction (expensive operation) failed, then we
		// delete the cached entry so that we may attempt to re-try again in the
		// future (instead of waiting for the LRUCache to evict it on its own
		// over time).
		//
		// TODO: If our cache implementation supports a TTL mechanism, then we
		// could just set that instead and let the cached entry to expire on its
		// own.
		if promise.err != nil {
			lruCache.Lock()
			_ = lruCache.Remove(key)
			lruCache.Unlock()
		}
	}

	return promise.val, promise.err
}
