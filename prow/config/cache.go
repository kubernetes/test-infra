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
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/v2"
)

// Caching implementation overview
//
// Consider the expensive function prowYAMLGetter(). This function is
// expensive to compute (it involves invoking a Git client, walking a filesystem
// path, etc). In our caching implementation, we save results of this function
// into a cache (named ProwYAMLCache). The point is to avoid doing the expensive
// GetPresubmits() and GetPostsubmits() calls if possible, which involves
// walking the repository to collect YAML information, etc.
//
// ProwYAMLCache uses an off-the-shelf LRU cache library for the low-level
// caching implementation, which uses the empty interface for keys and values.
// The values are what we store in the cache, and to retrieve them, we have to
// provide a key (which must be a hashable object). We wrap this cache with a
// single lock, and use an algorithm for a concurrent non-blocking cache to make
// it both thread-safe and also resistant to so-called cache stampede, where
// many concurrent threads all attempt to look up the same (missing) key/value
// pair from the cache (see Alan Donovan and Brian Kernighan, "The Go
// Programming Language" (Addison-Wesley, 2016), p. 277).

// ProwYAMLCache is the user-facing cache.
type ProwYAMLCache struct {
	LRUCache
}

// LRUCache is the actual concurrent non-blocking cache.
type LRUCache struct {
	*sync.Mutex
	*simplelru.LRU
}

// Promise is a wrapper around cache value construction; it is used to
// synchronize the to-be-cached value between threads that undergo a cache miss
// and subsequent threads that attempt to look up the same cache entry.
type Promise struct {
	Result
	resolve chan struct{}
}

// Result stores the result of executing an arbitrary valConstructor function.
type Result struct {
	val interface{}
	err error
}

// valConstructor is used to construct a value. The assumption is that this
// valConstructor is expensive to compute, and that we need to memoize it via
// the LRUCache. The raw values of a cache are only constructed after a cache
// miss or as a general fallback to bypass the cache if the cache is unusable
// for whatever reason. Using this type allows us to use any arbitrary function
// whose resulting values needs to be memoized (saved in the cache). This type
// also allows us to delay running the expensive computation until we actually
// need it.
type valConstructor func() (interface{}, error)

// keyConstructor is used only when we need to perform a lookup inside a cache
// (if it is available), because all values stored in the cache are paired with
// a unique lookup key.
type keyConstructor func() (CacheKey, error)

// NewLRUCache returns a new LRUCache with a given size (number of elements).
func NewLRUCache(size int) (*LRUCache, error) {
	cache, err := simplelru.NewLRU(size, nil)
	if err != nil {
		return nil, err
	}

	return &LRUCache{&sync.Mutex{}, cache}, nil
}

// NewProwYAMLCache creates a new LRU cache for ProwYAML values, where the keys
// are CacheKeys and values are pointers to ProwYAMLs.
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
func (c *Config) GetPresubmitsFromCache(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(c.ProwYAMLCache, c.getProwYAMLNoDefault, gc, identifier, baseSHAGetter, headSHAGetters...)
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
func (c *Config) GetPostsubmitsFromCache(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {

	prowYAML, err := GetProwYAMLFromCache(c.ProwYAMLCache, c.getProwYAMLNoDefault, gc, identifier, baseSHAGetter, headSHAGetters...)
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
// This cache is resistant to cache stampedes because it uses a duplicate
// suppression strategy. This is also called request coalescing.
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
