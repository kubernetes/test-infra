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
	"strings"
	"time"

	"k8s.io/test-infra/experiment/nursery/diskcache"

	log "github.com/sirupsen/logrus"
)

var dir = flag.String("dir", "", "location to store cache entries on disk")
var host = flag.String("host", "", "host address to listen on")
var port = flag.Int("port", 8080, "port to listen on")

func init() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
}

func main() {
	// TODO(bentheelder): bound cache size / convert to LRU
	// TODO(bentheelder): improve logging
	flag.Parse()
	if *dir == "" {
		log.Fatal("--dir must be set!")
	}

	cache := diskcache.NewCache(*dir)
	http.Handle("/", cacheHandler(cache))

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.WithError(http.ListenAndServe(addr, nil)).Fatal("ListenAndServe returned.")
}

// file not found error, used below
var errNotFound = errors.New("entry not found")

func cacheHandler(cache *diskcache.Cache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// parse and validate path
		// the last segment should be a hash, and the
		// the second to last segment should be "ac" or "cas"
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			log.Warn("received an invalid request at path: %v", r.URL.Path)
			http.Error(w, "invalid location", http.StatusBadRequest)
			return
		}
		hash := parts[len(parts)-1]
		acOrCAS := parts[len(parts)-2]
		if acOrCAS != "ac" && acOrCAS != "cas" {
			log.Warn("received an invalid request at path: %v", r.URL.Path)
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
				log.WithError(err).Error("error getting key")
				http.Error(w, err.Error(), http.StatusNotFound)
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
				log.WithError(err).Errorf("Failed to put: %v", r.URL.Path)
			}

		// handle unsupported methods...
		default:
			log.Warn("received an invalid request method: %v", r.Method)
			http.Error(w, "unsupported method", http.StatusBadRequest)
		}
	})
}
