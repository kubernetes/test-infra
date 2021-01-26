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

package main

import (
	"sort"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/greenhouse/diskcache"
	"k8s.io/test-infra/greenhouse/diskutil"
)

// monitorDiskAndEvict loops monitoring the disk, evicting cache entries
// when the disk passes either minPercentBlocksFree until the disk is above
// evictUntilPercentBlocksFree
func monitorDiskAndEvict(
	c *diskcache.Cache,
	interval time.Duration,
	minPercentBlocksFree, evictUntilPercentBlocksFree float64,
) {
	diskRoot := c.DiskRoot()
	// forever check if usage is past thresholds and evict
	ticker := time.NewTicker(interval)
	for ; true; <-ticker.C {
		blocksFree, _, _, err := diskutil.GetDiskUsage(diskRoot)
		if err != nil {
			logrus.WithError(err).Error("Failed to get disk usage!")
			continue
		}
		logger := logrus.WithFields(logrus.Fields{
			"sync-loop":   "MonitorDiskAndEvict",
			"blocks-free": blocksFree,
		})
		logger.Info("tick")
		// if we are past the threshold, start evicting
		if blocksFree < minPercentBlocksFree {
			logger.Warn("Eviction triggered")
			// get all cache entries and sort by lastaccess
			// so we can pop entries until we have evicted enough
			files := c.GetEntries()
			sort.Slice(files, func(i, j int) bool {
				return files[i].LastAccess.Before(files[j].LastAccess)
			})
			// evict until we pass the safe threshold so we don't thrash at the eviction trigger
			for blocksFree < evictUntilPercentBlocksFree {
				if len(files) < 1 {
					logger.Fatal("Failed to find entries to evict!")
				}
				// pop entry and delete
				var entry diskcache.EntryInfo
				entry, files = files[0], files[1:]
				err = c.Delete(c.PathToKey(entry.Path))
				if err != nil {
					logger.WithError(err).Errorf("Error deleting entry at path: %v", entry.Path)
				} else {
					promMetrics.FilesEvicted.Inc()
					promMetrics.LastEvictedAccessAge.Set(time.Since(entry.LastAccess).Hours())
				}
				// get new disk usage
				blocksFree, _, _, err = diskutil.GetDiskUsage(diskRoot)
				logger = logrus.WithFields(logrus.Fields{
					"sync-loop":   "MonitorDiskAndEvict",
					"blocks-free": blocksFree,
				})
				if err != nil {
					logrus.WithError(err).Error("Failed to get disk usage!")
					continue
				}
			}
			logger.Info("Done evicting")
		}
	}
}
