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

// greenhouse implements a bazel remote cache service [1]
// supporting arbitrarily many workspaces stored within the same
// top level directory.
//
// the first path segment in each {PUT,GET} request is mapped to an individual
// workspace cache, the remaining segments should follow [2].
//
// nursery assumes you are using SHA256
//
// [1] https://docs.bazel.build/versions/master/remote-caching.html
// [2] https://docs.bazel.build/versions/master/remote-caching.html#http-caching-protocol
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/test-infra/greenhouse/diskcache"
	"k8s.io/test-infra/greenhouse/diskutil"
	"k8s.io/test-infra/prow/logrusutil"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var dir = flag.String("dir", "", "location to store cache entries on disk")
var host = flag.String("host", "", "host address to listen on")
var cachePort = flag.Int("cache-port", 8080, "port to listen on for cache requests")
var metricsPort = flag.Int("metrics-port", 9090, "port to listen on for prometheus metrics scraping")
var metricsUpdateInterval = flag.Duration("metrics-update-interval", time.Second*10,
	"interval between updating disk metrics")

// eviction knobs
var minPercentBlocksFree = flag.Float64("min-percent-blocks-free", 5,
	"minimum percent of blocks free on --dir's disk before evicting entries")
var evictUntilPercentBlocksFree = flag.Float64("evict-until-percent-blocks-free", 20,
	"continue evicting from the cache until at least this percent of blocks are free")
var diskCheckInterval = flag.Duration("disk-check-interval", time.Second*10,
	"interval between checking disk usage (and potentially evicting entries)")

// global metrics object, see prometheus.go
var promMetrics *prometheusMetrics

func init() {
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "greenhouse"}),
	)
	logrus.SetOutput(os.Stdout)
	promMetrics = initMetrics()
}

func main() {
	flag.Parse()
	if *dir == "" {
		logrus.Fatal("--dir must be set!")
	}

	cache := diskcache.NewCache(*dir)
	go monitorDiskAndEvict(
		cache, *diskCheckInterval,
		*minPercentBlocksFree, *evictUntilPercentBlocksFree,
	)

	go updateMetrics(*metricsUpdateInterval, cache.DiskRoot())

	// listen for prometheus scraping
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/prometheus", promhttp.Handler())
	metricsAddr := fmt.Sprintf("%s:%d", *host, *metricsPort)
	go func() {
		logrus.Infof("Metrics Listening on: %s", metricsAddr)
		logrus.WithField("mux", "metrics").WithError(
			http.ListenAndServe(metricsAddr, metricsMux),
		).Fatal("ListenAndServe returned.")
	}()

	// listen for cache requests
	cacheMux := http.NewServeMux()
	cacheMux.Handle("/", cacheHandler(cache))
	cacheAddr := fmt.Sprintf("%s:%d", *host, *cachePort)
	logrus.Infof("Cache Listening on: %s", cacheAddr)
	logrus.WithField("mux", "cache").WithError(
		http.ListenAndServe(cacheAddr, cacheMux),
	).Fatal("ListenAndServe returned.")
}

// file not found error, used below
var errNotFound = errors.New("entry not found")

func cacheHandler(cache *diskcache.Cache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logrus.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
		})
		// parse and validate path
		// the last segment should be a hash, and
		// the second to last segment should be "ac" or "cas"
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			logger.Warn("received an invalid request")
			http.Error(w, "invalid location", http.StatusBadRequest)
			return
		}
		hash := parts[len(parts)-1]
		acOrCAS := parts[len(parts)-2]
		if acOrCAS != "ac" && acOrCAS != "cas" {
			logger.Warn("received an invalid request at path")
			http.Error(w, "invalid location", http.StatusBadRequest)
			return
		}
		requestingAction := acOrCAS == "ac"

		// actually handle request depending on method
		switch m := r.Method; m {
		// handle retrieval
		case http.MethodGet:
			err := cache.Get(r.URL.Path, func(exists bool, contents io.ReadSeeker) error {
				if !exists {
					return errNotFound
				}
				http.ServeContent(w, r, "", time.Time{}, contents)
				return nil
			})
			if err != nil {
				// file not present
				if err == errNotFound {
					if requestingAction {
						promMetrics.ActionCacheMisses.Inc()
					} else {
						promMetrics.CASMisses.Inc()
					}
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				// unknown error
				logger.WithError(err).Error("error getting key")
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// success, log hit
			if requestingAction {
				promMetrics.ActionCacheHits.Inc()
			} else {
				promMetrics.CASHits.Inc()
			}

		// handle upload
		case http.MethodPut:
			// only hash CAS, not action cache
			// the action cache is hash -> metadata
			// the CAS is well, a CAS, which we can hash...
			if requestingAction {
				hash = ""
			}
			err := cache.Put(r.URL.Path, r.Body, hash)
			if err != nil {
				logger.WithError(err).Errorf("Failed to put: %v", r.URL.Path)
				http.Error(w, "failed to put in cache", http.StatusInternalServerError)
				return
			}

		// handle unsupported methods...
		default:
			logger.Warn("received an invalid request method")
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		}
	})
}

// helper to update disk metrics
func updateMetrics(interval time.Duration, diskRoot string) {
	logger := logrus.WithField("sync-loop", "updateMetrics")
	ticker := time.NewTicker(interval)
	for ; true; <-ticker.C {
		logger.Info("tick")
		_, bytesFree, bytesUsed, err := diskutil.GetDiskUsage(diskRoot)
		if err != nil {
			logger.WithError(err).Error("Failed to get disk metrics")
		} else {
			promMetrics.DiskFree.Set(float64(bytesFree) / 1e9)
			promMetrics.DiskUsed.Set(float64(bytesUsed) / 1e9)
			promMetrics.DiskTotal.Set(float64(bytesFree+bytesUsed) / 1e9)
		}
	}
}
