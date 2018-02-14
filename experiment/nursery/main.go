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
	"os/exec"
	"strings"
	"time"

	"k8s.io/test-infra/experiment/nursery/diskcache"

	log "github.com/sirupsen/logrus"
)

var dir = flag.String("dir", "", "location to store cache entries on disk")
var host = flag.String("host", "", "host address to listen on")
var port = flag.Int("port", 8080, "port to listen on")

// NOTE: remount is a bit of a hack, unfortunately the kubernetes volumes
// don't really support this and to cleanly track entry access times we
// want to use a volume with lazyatime (and not noatime or relatime)
// so that file access times *are* recorded but are lazily flushed to the disk
var remount = flag.Bool("remount", false,
	"attempt to remount --dir with strictatime,lazyatime to improve eviction")

func init() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
}

func main() {
	// TODO(bentheelder): bound cache size / convert to LRU
	// TODO(bentheelder): improve logging
	flag.Parse()
	logger := log.WithFields(log.Fields{
		"component": "nursery",
	})
	if *dir == "" {
		logger.Fatal("--dir must be set!")
	}
	if *remount {
		device, mount, err := findMountForPath(*dir)
		if err != nil {
			logger.WithError(err).Errorf("Failed to find mountpoint for %s", *dir)
		} else {
			logger.Warnf(
				"Attempting to remount %s on %s with 'strictatime,lazyatime'",
				device, mount,
			)
			err = remountWithLazyAtime(device, mount)
			if err != nil {
				logger.WithError(err).Error("Failed to remount with lazyatime!")
			}
		}
	}

	cache := diskcache.NewCache(*dir)
	http.Handle("/", cacheHandler(cache))

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

/*
	--remount related code below
*/

// exec command and returns lines of combined output
func commandLines(cmd []string) (lines []string, err error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

// uses `mount -l` to find the device / mountpoint on which path is mounted
func findMountForPath(path string) (device, mountPoint string, err error) {
	mounts, err := commandLines([]string{"mount"})
	if err != nil {
		return "", "", err
	}
	// these lines are like:
	// $fs on $mountpoint type $fs_type ($mountopts)
	device, mountPoint = "", ""
	for _, mount := range mounts {
		parts := strings.Fields(mount)
		if len(parts) >= 3 {
			currDevice := parts[0]
			currMount := parts[2]
			// we want the longest matching mountpoint
			if strings.HasPrefix(path, currMount) && len(currMount) > len(mountPoint) {
				device, mountPoint = currDevice, currMount
			}
		}
	}
	return device, mountPoint, nil
}

// remounts with 'strictatime,lazyatime'
func remountWithLazyAtime(device, mountPoint string) error {
	_, err := commandLines([]string{
		"mount", "-o", "remount,strictatime,lazytime", device, mountPoint,
	})
	return err
}
