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

package cache

import (
	"fmt"
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/sirupsen/logrus"
)

// Overview
//
// LRUCache uses an off-the-shelf LRU cache library for the low-level
// caching implementation, which uses the empty interface for keys and values.
// The values are what we store in the cache, and to retrieve them, we have to
// provide a key (which must be a hashable object). We wrap this cache with a
// single lock, and use an algorithm for a concurrent non-blocking cache to make
// it both thread-safe and also resistant to so-called cache stampedes, where
// many concurrent threads all attempt to look up the same (missing) key/value
// pair from the cache (see Alan Donovan and Brian Kernighan, "The Go
// Programming Language" (Addison-Wesley, 2016), p. 277).

// LRUCache is the actual concurrent non-blocking cache.
type LRUCache struct {
	*sync.Mutex
	*simplelru.LRU
}

// ValConstructor is used to construct a value. The assumption is that this
// ValConstructor is expensive to compute, and that we need to memoize it via
// the LRUCache. The raw values of a cache are only constructed after a cache
// miss (and only the first cache miss). Using this type allows us to use any
// arbitrary function whose resulting value needs to be memoized (saved in the
// cache). This type also allows us to delay running the expensive computation
// until we actually need it (after a cache miss).
type ValConstructor func() (interface{}, error)

// Promise is a wrapper around cache value construction; it is used to
// synchronize the to-be-cached value between the first thread that undergoes a
// cache miss and subsequent threads that attempt to look up the same cache
// entry (cache hit). When the Promise is resolved (when the
// "valConstructionPending" channel is closed), the value is ready for
// concurrent reads.
type Promise struct {
	valConstructor         ValConstructor
	valConstructionPending chan struct{}
	val                    interface{}
	err                    error
}

func newPromise(valConstructor ValConstructor) *Promise {
	return &Promise{
		valConstructor:         valConstructor,
		valConstructionPending: make(chan struct{}),
	}
}

// waitForResolution blocks the current thread until the first thread that
// detected a cache miss has finished constructing the value (see resolve()).
func (p *Promise) waitForResolution() {
	<-p.valConstructionPending
}

// resolve resolves the Promise by constructing the value and closing the
// valConstructionPending channel, thereby unblocking any other thread that has
// been waiting for the value to be constructed.
func (p *Promise) resolve() {
	p.val, p.err = p.valConstructor()
	close(p.valConstructionPending)
}

// NewLRUCache returns a new LRUCache with a given size (number of elements).
func NewLRUCache(size int) (*LRUCache, error) {
	cache, err := simplelru.NewLRU(size, nil)
	if err != nil {
		return nil, err
	}

	return &LRUCache{&sync.Mutex{}, cache}, nil
}

// GetOrAdd tries to use a cache if it is available to get a Value. It is
// assumed that Value is expensive to construct from scratch, which is the
// reason why we try to use the cache in the first place. If we do end up
// constructing a Value from scratch, we store it into the cache with a
// corresponding key, so that we can look up the Value with just the key in the
// future.
//
// This cache is resistant to cache stampedes because it uses a duplicate
// suppression strategy. This is also called request coalescing.
func (lruCache *LRUCache) GetOrAdd(
	key interface{},
	valConstructor ValConstructor) (interface{}, error) {

	// Cache lookup.
	lruCache.Lock()
	var promise *Promise
	var ok bool
	maybePromise, promisePending := lruCache.Get(key)

	if promisePending {
		// A promise exists, BUT the wrapped value inside it (p.val) might
		// not be written to yet by the thread that is actually resolving the
		// promise.
		//
		// For now we just unlock the overall lruCache itself so that it can
		// service other GetOrAdd() calls to it.
		lruCache.Unlock()

		// If the type is not a Promise type, there's no need to wait and we can
		// just return immediately with an error.
		promise, ok = maybePromise.(*Promise)
		if !ok {
			err := fmt.Errorf("Programmer error: expected cache entry type '*Promise', got '%T'", maybePromise)
			logrus.WithField("key", key).Error(err)
			return nil, err
		}

		// Block until the first thread originally created this promise has
		// finished resolving it. Then it's safe to return the resolved values
		// of the promise below.
		//
		// If the original thread resolved the promise already a long time ago
		// (by calling resolve()), then this this waitForResolution() will
		// finish immediately and we will not block at all.
		promise.waitForResolution()
	} else {
		// No promise exists for this key. In other words, we are the first
		// thread to ask for this key's value and so We have no choice but to
		// construct the value ourselves (this call is expensive!) and add it to
		// the cache.
		//
		// If there are other concurrent threads that call GetOrAdd() with the
		// same key and corresponding value constructor, we force them to use
		// the same value as us (so that they don't have to also call
		// valConstructor()). We do this with the following algorithm:
		//
		//  1. immediately create a Promise to construct the value;
		//  2. actually construct the value (expensive operation);
		//  3. resolve the promise to alert all threads looking at the same Promise
		//     get the value from Step 2.
		//
		// This mitigation strategy is a kind of "duplicate suppression", also
		// called "request coalescing". The problem of having to deal with a
		// flood of multiple requests for the same cache entry is also called
		// "cache stampede".

		// Step 1
		//
		// Let other threads know about our promise to construct the value. We
		// don't care if the underlying LRU cache had to evict an existing
		// entry.
		promise = newPromise(valConstructor)
		_ = lruCache.Add(key, promise)
		// We must unlock here so that the cache does not block other GetOrAdd()
		// calls to it for different (or same) key/value pairs.
		lruCache.Unlock()

		// Step 2 & 3
		//
		// Construct the value (expensive operation), and broadcast to all
		// watchers of this promise that it is ready to be read from (no data
		// race!).
		promise.resolve()

		// If the value construction (expensive operation) failed, then we
		// delete the cached entry so that we may attempt to re-try again in the
		// future (instead of waiting for the LRUCache to evict it on its own
		// over time).
		//
		// NOTE: It may be the case that the underlying lruCache itself decided
		// to evict this key by the time we try to Lock() it here and evict it
		// ourselves. I.e., it may be the case that the lruCache evicted our key
		// because there just happened to be a massive load of calls with lots
		// of different keys, forcing all old cached values to be evicted. But
		// this is a minor concern because (1) it is unlikely to happen and (2)
		// even if it does happen, our eviction will be a NOP because the key we
		// want to delete wouldn't be in the cache anyway (it's already been
		// evicted!).
		//
		// Another possibility is that by the time we run attempt to delete the
		// key here, there has been not only an eviction of this same key, but
		// the creation of another entry with the same key with valid results.
		// So at worst we would be wrongfully invalidating a cache entry.
		//
		// TODO: If our cache implementation supports a TTL mechanism, then we
		// could just set that instead and let the cached entry expire on its
		// own (we would not have to do this eviction ourselves manually).
		if promise.err != nil {
			logrus.WithField("key", key).Infof("promise was successfully resolved, but the call to resolve() returned an error; deleting key from cache...")
			lruCache.Lock()
			keyWasFoundBeforeRemoval := lruCache.Remove(key)
			lruCache.Unlock()
			if keyWasFoundBeforeRemoval {
				logrus.WithField("key", key).Infof("successfully deleted")
			} else {
				err := fmt.Errorf("unexpected (non-problematic) race: key deleted by the cache without our knowledge; our own deletion of this key was a NOP but this does not constitute a problem")
				logrus.WithField("key", key).Info(err)
			}
		}
	}

	return promise.val, promise.err
}
