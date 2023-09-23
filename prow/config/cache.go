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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cache"
	"k8s.io/test-infra/prow/git/v2"
)

// Overview
//
// Consider the expensive function prowYAMLGetter(), which needs to use a Git
// client, walk the filesystem path, etc. To speed things up, we save results of
// this function into a cache named InRepoConfigCache.

var inRepoConfigCacheMetrics = struct {
	// How many times have we looked up an item in this cache?
	lookups *prometheus.CounterVec
	// Of the lookups, how many times did we get a cache hit?
	hits *prometheus.CounterVec
	// Of the lookups, how many times did we have to construct a cache value
	// ourselves (cache was useless for this lookup)?
	misses *prometheus.CounterVec
	// How many cache key evictions were performed by the underlying LRU
	// algorithm outside of our control?
	evictionsForced *prometheus.CounterVec
	// How many times have we tried to remove a cached key because its value
	// construction failed?
	evictionsManual *prometheus.CounterVec
	// How many entries are in the cache?
	cacheUsageSize *prometheus.GaugeVec
	// How long does it take for GetProwYAML() to run?
	getProwYAMLDuration *prometheus.HistogramVec
}{
	lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "inRepoConfigCache_lookups",
		Help: "Count of cache lookups by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	hits: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "inRepoConfigCache_hits",
		Help: "Count of cache lookup hits by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	misses: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "inRepoConfigCache_misses",
		Help: "Count of cache lookup misses by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	// Every time we evict a key, record it as a Prometheus metric. This way, we
	// can monitor how frequently evictions are happening (if it's happening too
	// frequently, it means that our cache size is too small).
	evictionsForced: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "inRepoConfigCache_evictions_forced",
		Help: "Count of forced cache evictions (due to LRU algorithm) by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	evictionsManual: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "inRepoConfigCache_evictions_manual",
		Help: "Count of manual cache evictions (due to faulty value construction) by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	cacheUsageSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "inRepoConfigCache_cache_usage_size",
		Help: "Size of the cache (how many entries it is holding) by org and repo.",
	}, []string{
		"org",
		"repo",
	}),
	getProwYAMLDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "inRepoConfigCache_GetProwYAML_duration",
		Help:    "Histogram of seconds spent retrieving the ProwYAML (inrepoconfig), by org and repo.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 120, 180, 300, 600},
	}, []string{
		"org",
		"repo",
	}),
}

func init() {
	prometheus.MustRegister(inRepoConfigCacheMetrics.lookups)
	prometheus.MustRegister(inRepoConfigCacheMetrics.hits)
	prometheus.MustRegister(inRepoConfigCacheMetrics.misses)
	prometheus.MustRegister(inRepoConfigCacheMetrics.evictionsForced)
	prometheus.MustRegister(inRepoConfigCacheMetrics.evictionsManual)
	prometheus.MustRegister(inRepoConfigCacheMetrics.cacheUsageSize)
	prometheus.MustRegister(inRepoConfigCacheMetrics.getProwYAMLDuration)
}

func mkCacheEventCallback(counterVec *prometheus.CounterVec) cache.EventCallback {
	callback := func(key interface{}) {
		org, repo, err := keyToOrgRepo(key)
		if err != nil {
			return
		}
		counterVec.WithLabelValues(org, repo).Inc()
	}

	return callback
}

// The InRepoConfigCache needs a Config agent client. Here we require that the Agent
// type fits the prowConfigAgentClient interface, which requires a Config()
// method to retrieve the current Config. Tests can use a fake Config agent
// instead of the real one.
var _ prowConfigAgentClient = (*Agent)(nil)

type prowConfigAgentClient interface {
	Config() *Config
}

// InRepoConfigCache is the user-facing cache. It acts as a wrapper around the
// generic LRUCache, by handling type casting in and out of the LRUCache (which
// only handles empty interfaces).
type InRepoConfigCache struct {
	*cache.LRUCache
	configAgent prowConfigAgentClient
	gitClient   git.ClientFactory
}

// NewInRepoConfigCache creates a new LRU cache for ProwYAML values, where the keys
// are CacheKeys (that is, JSON strings) and values are pointers to ProwYAMLs.
func NewInRepoConfigCache(
	size int,
	configAgent prowConfigAgentClient,
	gitClientFactory git.ClientFactory) (*InRepoConfigCache, error) {

	if gitClientFactory == nil {
		return nil, fmt.Errorf("InRepoConfigCache requires a non-nil gitClientFactory")
	}

	lookupsCallback := mkCacheEventCallback(inRepoConfigCacheMetrics.lookups)
	hitsCallback := mkCacheEventCallback(inRepoConfigCacheMetrics.hits)
	missesCallback := mkCacheEventCallback(inRepoConfigCacheMetrics.misses)
	forcedEvictionsCallback := func(key interface{}, _ interface{}) {
		org, repo, err := keyToOrgRepo(key)
		if err != nil {
			return
		}
		inRepoConfigCacheMetrics.evictionsForced.WithLabelValues(org, repo).Inc()
	}
	manualEvictionsCallback := mkCacheEventCallback(inRepoConfigCacheMetrics.evictionsManual)

	callbacks := cache.Callbacks{
		LookupsCallback:         lookupsCallback,
		HitsCallback:            hitsCallback,
		MissesCallback:          missesCallback,
		ForcedEvictionsCallback: forcedEvictionsCallback,
		ManualEvictionsCallback: manualEvictionsCallback,
	}

	lruCache, err := cache.NewLRUCache(size, callbacks)
	if err != nil {
		return nil, err
	}

	// This records all OrgRepos we've seen so far during the lifetime of the
	// process. The main purpose is to allow reporting of 0 counts for OrgRepos
	// whose keys have been evicted by the lruCache.
	seenOrgRepos := make(map[OrgRepo]int)

	cacheSizeMetrics := func() {
		lruCache.Mutex.Lock()         // Lock the mutex
		defer lruCache.Mutex.Unlock() // Unlock the mutex when done
		// Record all unique orgRepo combinations we've seen so far.
		for _, key := range lruCache.Keys() {
			org, repo, err := keyToOrgRepo(key)
			if err != nil {
				// This should only happen if we are deliberately using things
				// other than a CacheKey as the key.
				logrus.Warnf("programmer error: could not report cache size metrics for a key entry: %v", err)
				continue
			}
			orgRepo := OrgRepo{org, repo}
			if count, ok := seenOrgRepos[orgRepo]; ok {
				seenOrgRepos[orgRepo] = count + 1
			} else {
				seenOrgRepos[orgRepo] = 1
			}
		}
		// For every single org and repo in the cache, report how many key
		// entries there are.
		for orgRepo, count := range seenOrgRepos {
			inRepoConfigCacheMetrics.cacheUsageSize.WithLabelValues(
				orgRepo.Org, orgRepo.Repo).Set(float64(count))
			// Reset the counter back down to 0 because it may be that by the
			// time of the next interval, the last key for this orgRepo will be
			// evicted. At that point we still want to report a count of 0.
			seenOrgRepos[orgRepo] = 0
		}
	}

	go func() {
		for {
			cacheSizeMetrics()
			time.Sleep(30 * time.Second)
		}
	}()

	cache := &InRepoConfigCache{
		lruCache,
		// Know how to default the retrieved ProwYAML values against the latest Config.
		configAgent,
		// Make the cache be able to handle cache misses (by calling out to Git
		// to construct the ProwYAML value).
		gitClientFactory,
	}

	return cache, nil
}

// CacheKey acts as a key to the InRepoConfigCache. We construct it by marshaling
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

// CacheKey converts a CacheKeyParts object into a JSON string (to be used as a
// CacheKey).
func (kp *CacheKeyParts) CacheKey() (CacheKey, error) {
	data, err := json.Marshal(kp)
	if err != nil {
		return "", err
	}

	return CacheKey(data), nil
}

func (cacheKey CacheKey) toCacheKeyParts() (CacheKeyParts, error) {
	kp := CacheKeyParts{}
	if err := json.Unmarshal([]byte(cacheKey), &kp); err != nil {
		return kp, err
	}
	return kp, nil
}

func keyToOrgRepo(key interface{}) (string, string, error) {

	cacheKey, ok := key.(CacheKey)
	if !ok {
		return "", "", fmt.Errorf("key is not a CacheKey")
	}

	kp, err := cacheKey.toCacheKeyParts()
	if err != nil {
		return "", "", err
	}

	org, repo, err := SplitRepoName(kp.Identifier)
	if err != nil {
		return "", "", err
	}

	return org, repo, nil
}

// GetPresubmits uses a cache lookup to get the *ProwYAML value (cache hit),
// instead of computing it from scratch (cache miss). It also stores the
// *ProwYAML into the cache if there is a cache miss.
func (cache *InRepoConfigCache) GetPresubmits(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {
	prowYAML, err := cache.GetProwYAML(identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	c := cache.configAgent.Config()
	return append(c.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

// GetPostsubmitsCached is like GetPostsubmits, but attempts to use a cache
// lookup to get the *ProwYAML value (cache hit), instead of computing it from
// scratch (cache miss). It also stores the *ProwYAML into the cache if there is
// a cache miss.
func (cache *InRepoConfigCache) GetPostsubmits(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {
	prowYAML, err := cache.GetProwYAML(identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	c := cache.configAgent.Config()
	return append(c.GetPostsubmitsStatic(identifier), prowYAML.Postsubmits...), nil
}

// GetProwYAML returns the ProwYAML value stored in the InRepoConfigCache.
func (cache *InRepoConfigCache) GetProwYAML(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	prowYAML, err := cache.GetProwYAMLWithoutDefaults(identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	c := cache.configAgent.Config()

	// Create a new ProwYAML object based on what we retrieved from the cache.
	// This way, the act of defaulting values does not modify the elements in
	// the Presubmits and Postsubmits slices (recall that slices are just
	// references to areas of memory). This is important for InRepoConfigCache to
	// behave correctly; otherwise when we default the cached ProwYAML values,
	// the cached item becomes mutated, affecting future cache lookups.
	newProwYAML := prowYAML.DeepCopy()
	if err := DefaultAndValidateProwYAML(c, newProwYAML, identifier); err != nil {
		return nil, err
	}

	return newProwYAML, nil
}

func (cache *InRepoConfigCache) GetProwYAMLWithoutDefaults(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	timeGetProwYAML := time.Now()
	defer func() {
		orgRepo := NewOrgRepo(identifier)
		inRepoConfigCacheMetrics.getProwYAMLDuration.WithLabelValues(orgRepo.Org, orgRepo.Repo).Observe((float64(time.Since(timeGetProwYAML).Seconds())))
	}()

	c := cache.configAgent.Config()

	prowYAML, err := cache.getProwYAML(c.getProwYAML, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	return prowYAML, nil
}

// GetInRepoConfig just wraps around GetProwYAML().
func (cache *InRepoConfigCache) GetInRepoConfig(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	return cache.GetProwYAML(identifier, baseSHAGetter, headSHAGetters...)
}

// getProwYAML performs a lookup of previously-calculated *ProwYAML objects. The
// 'valConstructorHelper' is used in two ways. First it is used by the caching
// mechanism to lazily generate the value only when it is required (otherwise,
// if all threads had to generate the value, it would defeat the purpose of the
// cache in the first place). Second, it makes it easier to test this function,
// because unit tests can just provide its own function for constructing a
// *ProwYAML object (instead of needing to create an actual Git repo, etc.).
func (cache *InRepoConfigCache) getProwYAML(
	valConstructorHelper func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error),
	identifier string,
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (*ProwYAML, error) {

	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}

	// Abort if the InRepoConfig is not enabled for this identifier (org/repo).
	// It's important that we short-circuit here __before__ calling cache.Get()
	// because we do NOT want to add an empty &ProwYAML{} value in the cache
	// (because not only is it useless, but adding a useless entry also may
	// result in evicting a useful entry if the underlying cache is full and an
	// older (useful) key is evicted).
	c := cache.configAgent.Config()
	if !c.InRepoConfigEnabled(identifier) {
		logrus.WithField("identifier", identifier).Debug("Inrepoconfig not enabled, skipping getting prow yaml.")
		return &ProwYAML{}, nil
	}

	baseSHA, headSHAs, err := GetAndCheckRefs(baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	valConstructor := func() (interface{}, error) {
		return valConstructorHelper(cache.gitClient, identifier, baseSHAGetter, headSHAGetters...)
	}

	got, err := cache.get(CacheKeyParts{Identifier: identifier, BaseSHA: baseSHA, HeadSHAs: headSHAs}, valConstructor)
	if err != nil {
		return nil, err
	}

	return got, err
}

// get is a type assertion wrapper around the values retrieved from the inner
// LRUCache object (which only understands empty interfaces for both keys and
// values). It wraps around the low-level GetOrAdd function. Users are expected
// to add their own get method for their own cached value.
func (cache *InRepoConfigCache) get(
	keyParts CacheKeyParts,
	valConstructor cache.ValConstructor) (*ProwYAML, error) {

	key, err := keyParts.CacheKey()
	if err != nil {
		return nil, fmt.Errorf("converting CacheKeyParts to CacheKey: %v", err)
	}

	now := time.Now()
	val, cacheHit, err := cache.GetOrAdd(key, valConstructor)
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"identifier":        keyParts.Identifier,
		"key":               key,
		"duration(seconds)": -time.Until(now).Seconds(),
		"cache_hit":         cacheHit,
	}).Debug("Duration for resolving inrepoconfig cache.")

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
