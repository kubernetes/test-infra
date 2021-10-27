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

// Package diskutil implements disk related utilities for greenhouse
package diskutil

import (
	"syscall"
	"time"

	"github.com/djherbis/atime"
	log "github.com/sirupsen/logrus"
)

// GetDiskUsage wraps syscall.Statfs for usage in GCing the disk
func GetDiskUsage(path string) (percentBlocksFree float64, bytesFree, bytesUsed uint64, percentInodeFree float64, inodeFree, inodeUsed uint64, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	percentBlocksFree = float64(stat.Bfree) / float64(stat.Blocks) * 100
	bytesFree = stat.Bfree * uint64(stat.Bsize)
	bytesUsed = (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)

	percentInodeFree = float64(stat.Ffree) / float64(stat.Files) * 100
	inodeUsed = stat.Files - stat.Ffree
	return percentBlocksFree, bytesFree, bytesUsed, percentInodeFree, stat.Ffree, inodeUsed, nil
}

// GetATime the atime for a file, logging errors instead of failing
// and returning defaultTime instead
func GetATime(path string, defaultTime time.Time) time.Time {
	at, err := atime.Stat(path)
	if err != nil {
		log.WithError(err).Errorf("Could not get atime for %s", path)
		return defaultTime
	}
	return at
}
