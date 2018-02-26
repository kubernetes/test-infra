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
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/djherbis/atime"
	log "github.com/sirupsen/logrus"
)

// exec command and returns lines of combined output
func commandLines(cmd []string) (lines []string, err error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

// FindMountForPath wraps `mount` to find the device / mountpoint
// on which path is mounted
func FindMountForPath(path string) (device, mountPoint string, err error) {
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

// Remount `wraps mount -o remount,options device mountPoint`
func Remount(device, mountPoint, options string) error {
	_, err := commandLines([]string{
		"mount", "-o", fmt.Sprintf("remount,%s", options), device, mountPoint,
	})
	return err
}

// GetDiskUsage wraps syscall.Statfs for usage in GCing the disk
func GetDiskUsage(path string) (percentBlocksFree float64, bytesFree, bytesUsed uint64, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, 0, err
	}
	percentBlocksFree = float64(stat.Bfree) / float64(stat.Blocks) * 100
	bytesFree = stat.Bfree * uint64(stat.Bsize)
	bytesUsed = (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)
	return percentBlocksFree, bytesFree, bytesUsed, nil
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
