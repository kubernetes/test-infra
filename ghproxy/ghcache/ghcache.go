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

// Package ghcache implements an HTTP cache optimized for caching responses
// from the GitHub API (https://api.github.com).
//
// Specifically, it enforces a cache policy that revalidates every cache hit
// with a conditional request to upstream regardless of cache entry freshness
// because conditional requests for unchanged resources don't cost any API
// tokens!!! See: https://developer.github.com/v3/#conditional-requests
//
// It also provides request coalescing and prometheus instrumentation.
package ghcache

import (
	"context"
	"crypto/sha256"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
	rediscache "github.com/gregjones/httpcache/redis"
	"github.com/peterbourgon/diskv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"k8s.io/test-infra/ghproxy/ghmetrics"
)

type CacheResponseMode string

// Cache response modes describe how ghcache fulfilled a request.
const (
	CacheModeHeader = "X-Cache-Mode"

	ModeError   CacheResponseMode = "ERROR"    // internal error handling request
	ModeNoStore CacheResponseMode = "NO-STORE" // response not cacheable
	ModeMiss    CacheResponseMode = "MISS"     // not in cache, request proxied and response cached.
	ModeChanged CacheResponseMode = "CHANGED"  // cache value invalid: resource changed, cache updated
	// The modes below are the happy cases in which the request is fulfilled for
	// free (no API tokens used).
	ModeCoalesced   CacheResponseMode = "COALESCED"   // coalesced request, this is a copied response
	ModeRevalidated CacheResponseMode = "REVALIDATED" // cached value revalidated and returned
)

func CacheModeIsFree(mode CacheResponseMode) bool {
	switch mode {
	case ModeCoalesced:
		return true
	case ModeRevalidated:
		return true
	case ModeError:
		// In this case we did not successfully communicate with the GH API, so no
		// token is used, but we also don't return a response, so ModeError won't
		// ever be returned as a value of CacheModeHeader.
		return true
	}
	return false
}

// cacheCounter provides the 'ghcache_responses' counter vec that is indexed
// by the cache response mode.
var cacheCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "ghcache_responses",
		Help: "How many cache responses of each cache response mode there are.",
	},
	[]string{"mode"},
)

// outboundConcurrencyGauge provides the 'concurrent_outbound_requests' gauge that
// is global to the proxy.
var outboundConcurrencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "concurrent_outbound_requests",
	Help: "How many concurrent requests are in flight to GitHub servers.",
})

// pendingOutboundConnectionsGauge provides the 'pending_outbound_requests' gauge that
// is global to the proxy.
var pendingOutboundConnectionsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "pending_outbound_requests",
	Help: "How many pending requests are waiting to be sent to GitHub servers.",
})

func init() {
	prometheus.MustRegister(cacheCounter)
	prometheus.MustRegister(outboundConcurrencyGauge)
	prometheus.MustRegister(pendingOutboundConnectionsGauge)
}

func cacheResponseMode(headers http.Header) CacheResponseMode {
	if strings.Contains(headers.Get("Cache-Control"), "no-store") {
		return ModeNoStore
	}
	if strings.Contains(headers.Get("Status"), "304 Not Modified") {
		return ModeRevalidated
	}
	if headers.Get("X-Conditional-Request") != "" {
		return ModeChanged
	}
	return ModeMiss
}

func newThrottlingTransport(maxConcurrency int, delegate http.RoundTripper) http.RoundTripper {
	return &throttlingTransport{sem: semaphore.NewWeighted(int64(maxConcurrency)), delegate: delegate}
}

// throttlingTransport throttles outbound concurrency from the proxy
type throttlingTransport struct {
	sem      *semaphore.Weighted
	delegate http.RoundTripper
}

func (c *throttlingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	pendingOutboundConnectionsGauge.Inc()
	if err := c.sem.Acquire(context.Background(), 1); err != nil {
		logrus.WithField("cache-key", req.URL.String()).WithError(err).Error("Internal error acquiring semaphore.")
		return nil, err
	}
	defer c.sem.Release(1)
	pendingOutboundConnectionsGauge.Dec()
	outboundConcurrencyGauge.Inc()
	defer outboundConcurrencyGauge.Dec()
	return c.delegate.RoundTrip(req)
}

// upstreamTransport changes response headers from upstream before they
// reach the cache layer in order to force the caching policy we require.
//
// By default github responds to PR requests with:
//    Cache-Control: private, max-age=60, s-maxage=60
// Which means the httpcache would not consider anything stale for 60 seconds.
// However, we want to always revalidate cache entries using ETags and last
// modified times so this RoundTripper overrides response headers to:
//    Cache-Control: no-cache
// This instructs the cache to store the response, but always consider it stale.
type upstreamTransport struct {
	delegate http.RoundTripper
}

func (u upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	etag := req.Header.Get("if-none-match")

	// get authorization header to convert to sha256
	authHeader := req.Header.Get("Authorization")
	hasher := sha256.New()
	hasher.Write([]byte(authHeader))
	authHeaderHash := string(hasher.Sum(nil))
	if authHeader == "" {
		logrus.Warnf("Couldn't retrieve 'Authorization' header, %s is the hash of an empty string", authHeaderHash)
	}

	reqStartTime := time.Now()
	// Don't modify request, just pass to delegate.
	resp, err := u.delegate.RoundTrip(req)
	if err != nil {
		logrus.WithField("cache-key", req.URL.String()).WithError(err).Error("Error from upstream (GitHub).")
		return nil, err
	}
	responseTime := time.Now()

	if resp.StatusCode >= 400 {
		// Don't store errors. They can't be revalidated to save API tokens.
		resp.Header.Set("Cache-Control", "no-store")
	} else {
		resp.Header.Set("Cache-Control", "no-cache")
	}
	if etag != "" {
		resp.Header.Set("X-Conditional-Request", etag)
	}

	apiVersion := "v3"
	if strings.HasPrefix(req.URL.Path, "search") || strings.HasPrefix(req.URL.Path, "/search") {
		apiVersion = "v4"
	}

	ghmetrics.CollectGitHubTokenMetrics(authHeaderHash, apiVersion, resp.Header, reqStartTime, responseTime)

	return resp, nil
}

// NewDiskCache creates a GitHub cache RoundTripper that is backed by a disk
// cache.
func NewDiskCache(delegate http.RoundTripper, cacheDir string, cacheSizeGB, maxConcurrency int) http.RoundTripper {
	return NewFromCache(delegate, diskcache.NewWithDiskv(
		diskv.New(diskv.Options{
			BasePath:     path.Join(cacheDir, "data"),
			TempDir:      path.Join(cacheDir, "temp"),
			CacheSizeMax: uint64(cacheSizeGB) * uint64(1000000000), // convert G to B
		})),
		maxConcurrency,
	)
}

// NewMemCache creates a GitHub cache RoundTripper that is backed by a memory
// cache.
func NewMemCache(delegate http.RoundTripper, maxConcurrency int) http.RoundTripper {
	return NewFromCache(delegate, httpcache.NewMemoryCache(), maxConcurrency)
}

// NewFromCache creates a GitHub cache RoundTripper that is backed by the
// specified httpcache.Cache implementation.
func NewFromCache(delegate http.RoundTripper, cache httpcache.Cache, maxConcurrency int) http.RoundTripper {
	cacheTransport := httpcache.NewTransport(cache)
	cacheTransport.Transport = newThrottlingTransport(maxConcurrency, upstreamTransport{delegate: delegate})
	return &requestCoalescer{
		keys:     make(map[string]*responseWaiter),
		delegate: cacheTransport,
	}
}

// NewRedisCache creates a GitHub cache RoundTripper that is backed by a Redis
// cache.
func NewRedisCache(delegate http.RoundTripper, redisAddress string, maxConcurrency int) http.RoundTripper {
	conn, err := redis.Dial("tcp", redisAddress)
	if err != nil {
		logrus.WithError(err).Fatal("Error connecting to Redis")
	}
	return NewFromCache(delegate, rediscache.NewWithClient(conn), maxConcurrency)
}
