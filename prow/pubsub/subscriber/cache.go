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

package subscriber

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
)

// Caching implementation overview
//
// Consider the expensive function config.GetPresubmits(). This function returns
// a []config.Presubmit slice, which is expensive to compute (it involves
// invoking a Git client, walking a filesystem path, etc). In our caching
// implementation, we call a wrapper function instead, GetPresubmitsFromCache().
// GetPresubmitsFromCache() can wrap around config.GetPresubmits(), and __only__
// calls it when the corresponding []config.Presubmit slice we want is not found
// in the cache (or if the cache is not initialized properly, or even
// corrupted).
//
// The same thing can be said for config.GetPostsubmits() and its wrapper,
// GetPostsubmitsFromCache() which we define in this file.
//
// The key idea is this: when we need to look up a value for the cache, we first
// look it up by its key. The function that creates a key ("keyConstructor"
// type) is fast and easy to compute. If we find an entry in the cache that
// matches the key that we just constructed, then return that. Otherwise if the
// value is not found, we (begrudgingly) compute it from scratch using the given
// "value constructor" function (aka the "valConstructor" type). For
// []config.Presubmit the constructor function is config.GetPresubmits(), and
// for []config.Postsubmit it is config.GetPostsubmits().
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
// are the []config.Presubmit and []config.Postsubmit objects). The point is to
// avoid doing the expensive config.GetPresubmits() and config.GetPostsubmits()
// calls if possible, which involves walking the repository to collect YAML
// information, etc.
//
// We use an off-the-shelf LRU cache library for the low-level caching
// implementation, which uses the empty interface for keys and values. The
// values are what we store in the cache, and to retrieve them, we have to
// provide a key (which must be a hashable object). Because of Golang's lack of
// generics, the values retrieved from the cache must be type-asserted into the
// type we want ([]config.Presubmit or []config.Postsubmit) before we can use
// them.
type ProwYAMLCache struct {
	presubmits  *lru.Cache
	postsubmits *lru.Cache
}

// NewProwYAMLCache creates a new LRU cache for presubmits and postsubmits,
// where the keys are CacheKeys and values are ProwYAMLs.
func NewProwYAMLCache(size int) (*ProwYAMLCache, error) {
	presubmits, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	postsubmits, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	prowYAMLCache := &ProwYAMLCache{
		presubmits:  presubmits,
		postsubmits: postsubmits,
	}

	return prowYAMLCache, nil
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
type CacheKey struct {
	identifier string
	baseSHA    string
	headSHAs   []string
}

// This implements the fmt.Stringer interface for CacheKey.
func (k CacheKey) String() string {
	s := "identifier:" + k.identifier
	s += ",baseSHA:" + k.baseSHA
	for _, headSHA := range k.headSHAs {
		s += ",headSHA:" + headSHA
	}

	return s
}

// MakeCacheKey constructs a CacheKey type from uniquely-identifying
// information. The only requirement is that we take all of the stringlike
// parameters and concatenate them together to form a UUID. However to make
// debugging easier, we
func MakeCacheKey(
	identifier string,
	baseSHAGetter config.RefGetter,
	headSHAGetters ...config.RefGetter) (CacheKey, error) {
	// Initialize empty key.
	key := CacheKey{}

	// Append "identifier" string information.
	if identifier == "" {
		return CacheKey{}, fmt.Errorf("identifier cannot be empty")
	}
	key.identifier = identifier

	// Append "baseSHA" string information.
	baseSHA, err := baseSHAGetter()
	if err != nil {
		return CacheKey{}, fmt.Errorf("failed to get baseSHA: %v", err)
	}
	key.baseSHA = baseSHA

	// Append "headSHAs" string information.
	var headSHAs []string
	for _, headSHAGetter := range headSHAGetters {
		headSHA, err := headSHAGetter()
		if err != nil {
			return CacheKey{}, fmt.Errorf("failed to get headRef: %v", err)
		}
		headSHAs = append(headSHAs, headSHA)
	}
	key.headSHAs = headSHAs

	return key, err
}

// valConstructor is used to construct a value. The raw values of a cache are
// only constructed after a cache miss or as a general fallback to bypass the
// cache if the cache is unusable for whatever reason.
type valConstructor func() (interface{}, error)

// keyConstructor is used only when we need to perform a lookup inside a cache
// (if it is available), because all values stored in the cache are paired with
// a unique lookup key.
type keyConstructor func() (fmt.Stringer, error)

// GetPresubmitsFromCache uses ProwYAMLCache to first try to perform a lookup of
// previously-calculated []config.Presubmit objects. The 'vg' function is taken
// as an argument to make it easier to test this function. This way, unit tests
// can just provide its own function for constructing a []config.Presubmit
// object (instead of needing to create an actual Git repo, etc., as required by
// the config.GetPresubmits function).
func (p *ProwYAMLCache) GetPresubmitsFromCache(
	vg func(git.ClientFactory, string, config.RefGetter, ...config.RefGetter) ([]config.Presubmit, error),
	gc git.ClientFactory,
	identifier string,
	baseSHAGetter config.RefGetter,
	headSHAGetters ...config.RefGetter) ([]config.Presubmit, bool, bool, error) {

	keyConstructor := func() (fmt.Stringer, error) {
		return MakeCacheKey(identifier, baseSHAGetter, headSHAGetters...)
	}

	valConstructor := func() (interface{}, error) {
		return vg(gc, identifier, baseSHAGetter, headSHAGetters...)
	}

	val, cacheHit, evicted, err := GetFromCache(p.presubmits, keyConstructor, valConstructor)
	if err != nil {
		return nil, cacheHit, evicted, err
	}

	if presubmits, ok := val.([]config.Presubmit); ok {
		return presubmits, cacheHit, evicted, err
	}

	// Somehow, the value retrieved with GetFromCache has a malformed type. This
	// can happen if some other function modified the cache. Ultimately, this is
	// a price we pay for using a cache library that uses "interface{}" for the
	// type of its items. In this case, we heal the cache by deleting the cached
	// value because it is of an invalid type, and insert a valid one after
	// constructing it from scratch.
	presubmits, err := vg(gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, false, false, err
	}
	key, err := keyConstructor()
	if err != nil {
		return nil, false, false, err
	}
	evicted = p.presubmits.Add(key.String(), presubmits)

	return presubmits, false, evicted, nil
}

// GetPostsubmitsFromCache is virtually identical to GetPresubmitsFromCache. The
// only real difference is in the keyConstructor (postsubmits don't consider
// headSHAGetters).
func (p *ProwYAMLCache) GetPostsubmitsFromCache(
	vg func(git.ClientFactory, string, config.RefGetter, ...config.RefGetter) ([]config.Postsubmit, error),
	gc git.ClientFactory,
	identifier string,
	baseSHAGetter config.RefGetter) ([]config.Postsubmit, bool, bool, error) {

	keyConstructor := func() (fmt.Stringer, error) {
		return MakeCacheKey(identifier, baseSHAGetter)
	}

	valConstructor := func() (interface{}, error) {
		return vg(gc, identifier, baseSHAGetter)
	}

	val, cacheHit, evicted, err := GetFromCache(p.postsubmits, keyConstructor, valConstructor)
	if err != nil {
		return nil, cacheHit, evicted, err
	}

	if postsubmits, ok := val.([]config.Postsubmit); ok {
		return postsubmits, cacheHit, evicted, err
	}

	postsubmits, err := vg(gc, identifier, baseSHAGetter)
	if err != nil {
		return nil, false, false, err
	}
	key, err := keyConstructor()
	if err != nil {
		return nil, false, false, err
	}
	evicted = p.presubmits.Add(key.String(), postsubmits)

	return postsubmits, false, evicted, nil
}

// GetFromCache tries to use a cache if it is available to get a Value. It is
// assumed that Value is expensive to construct from scratch, which is the
// reason why we try to use the cache in the first place. If we do end up
// constructing a Value from scratch, then we store it into the cache with a
// corresponding Key, so that we can look up the Value with just the Key in the
// future.
func GetFromCache(
	cache *lru.Cache,
	keyConstructor keyConstructor,
	valConstructor valConstructor) (interface{}, bool, bool, error) {

	// If the cache is unreachable, then fall back to cache-less behavior
	// (construct the value from scratch).
	if cache == nil {
		valConstructed, err := valConstructor()
		if err != nil {
			return nil, false, false, err
		}

		return valConstructed, false, false, nil
	}

	// Construct cache key. We use this key to find the value (if it was already
	// stored in the cache by a previous call to GetFromCache).
	key, err := keyConstructor()
	if err != nil {
		return nil, false, false, err
	}

	// Cache lookup.
	valFound, ok := cache.Get(key.String())

	// Cache hit.
	if ok {
		return valFound, true, false, nil
	}

	// Cache miss. We have no choice but to construct the value (this call is
	// expensive!) and add it to the cache.
	valConstructed, err := valConstructor()
	if err != nil {
		return nil, false, false, err
	}

	// Add our constructed value to the cache.
	evicted := cache.Add(key.String(), valConstructed)

	return valConstructed, false, evicted, nil
}
