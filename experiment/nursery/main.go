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

// nursery implements a bazel remote cache service [1]
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
	"sort"
	"strings"
	"time"

	"k8s.io/test-infra/experiment/nursery/diskcache"
	"k8s.io/test-infra/experiment/nursery/diskutil"

	log "github.com/sirupsen/logrus"
)

var dir = flag.String("dir", "", "location to store cache entries on disk")
var host = flag.String("host", "", "host address to listen on")
var port = flag.Int("port", 8080, "port to listen on")

// eviction knobs
var minPercentBlocksFree = flag.Float64("min-percent-blocks-free", 10,
	"minimum percent of blocks free on --dir's disk before evicting entries")
var minPercentFilesFree = flag.Float64("min-percent-files-free", 10,
	"minimum percent of blocks free on --dir's disk before evicting entries")
var diskCheckInterval = flag.Duration("disk-check-interval", time.Minute,
	"interval between checking disk usage (and potentially evicting entries)")

// NOTE: remount is a bit of a hack, unfortunately the kubernetes volumes
// don't really support this and to cleanly track entry access times we
// want to use a volume with lazyatime (and not noatime or relatime)
// so that file access times *are* recorded but are lazily flushed to the disk
var remount = flag.Bool("remount", false,
	"attempt to remount --dir with strictatime,lazyatime to improve eviction")

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
}

func main() {
	// TODO(bentheelder): add metrics
	flag.Parse()
	logger := log.WithFields(log.Fields{
		"component": "nursery",
	})
	if *dir == "" {
		logger.Fatal("--dir must be set!")
	}
	if *remount {
		device, mount, err := diskutil.FindMountForPath(*dir)
		if err != nil {
			logger.WithError(err).Errorf("Failed to find mountpoint for %s", *dir)
		} else {
			logger.Warnf(
				"Attempting to remount %s on %s with 'strictatime,lazyatime'",
				device, mount,
			)
			err = diskutil.Remount(device, mount, "strictatime,lazytime")
			if err != nil {
				logger.WithError(err).Error("Failed to remount with lazyatime!")
			}
			logger.Info("Remount complete")
		}
	}

	cache := diskcache.NewCache(*dir)
	http.Handle("/", cacheHandler(cache))

	go monitorDiskAndEvict(cache, *diskCheckInterval, *minPercentBlocksFree, *minPercentFilesFree)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	logger.Infof("Listening on: %s", addr)
	logger.WithError(http.ListenAndServe(addr, nil)).Fatal("ListenAndServe returned.")
}

// file not found error, used below
var errNotFound = errors.New("entry not found")

func cacheHandler(cache *diskcache.Cache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.WithFields(log.Fields{
			"component": "nursery",
			"method":    r.Method,
			"path":      r.URL.Path,
		})
		// parse and validate path
		// the last segment should be a hash, and the
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

		// actually handle request depending on method
		switch m := r.Method; m {
		// handle retreival
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
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				// unknown error
				logger.WithError(err).Error("error getting key")
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

		// handle upload
		case http.MethodPut:
			// only hash CAS, not action cache
			// the action cache is hash -> metadata
			// the CAS is well, a CAS, which we can hash...
			if acOrCAS != "cas" {
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

func monitorDiskAndEvict(cache *diskcache.Cache, interval time.Duration, minBlocksFree, minFilesFree float64) {
	logger := log.WithFields(log.Fields{
		"component": "nursery",
	})
	dir := cache.DiskRoot()
	// forever check if usage is past thresholds and evict
	for range time.Tick(interval) {
		blocksFree, filesFree := diskutil.GetDiskUsage(dir)
		// if we are past either threshold, evict until we are not
		if blocksFree < minBlocksFree || filesFree < minFilesFree {
			logger.WithFields(log.Fields{
				"blocks-free": blocksFree,
				"files-free":  filesFree,
			}).Warn("Eviction triggered")
			// get all cache entries and sort by lastaccess
			// so we can pop entries until we have evicted enough
			files := cache.GetEntries()
			sort.Slice(files, func(i, j int) bool {
				return files[i].LastAccess.After(files[j].LastAccess)
			})
			// actual eviction loop occurs here
			for blocksFree < minBlocksFree || filesFree < minFilesFree {
				if len(files) < 1 {
					logger.Fatalf("Failed to find entries to evict!")
				}
				// pop entry and delete
				var entry diskcache.EntryInfo
				entry, files = files[0], files[1:]
				err := cache.Delete(cache.PathToKey(entry.Path))
				if err != nil {
					logger.WithError(err).Error("Error deleting entry")
				}
				// get new disk usage
				blocksFree, filesFree = diskutil.GetDiskUsage(dir)
			}
		}
	}
}
