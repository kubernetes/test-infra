/*
Copyright 2020 The Kubernetes Authors.

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

package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// Caches is the basic abstraction over multiple Cache entries.
type Caches []*Cache

// Validate verifies if every Cache contains a path and keys.
func (c Caches) Validate() error {
	unique := make(map[string]bool, len(c))

	for _, cache := range c {
		if cache.Path == "" {
			return errors.Errorf("cache contains no path")
		}

		if !unique[cache.Path] {
			unique[cache.Path] = true
		} else {
			return errors.Errorf("cache path %q is not unique in all caches", cache.Path)
		}

		if !filepath.IsAbs(cache.Path) {
			return errors.Errorf("cache path %q is not absolute", cache.Path)
		}

		if len(cache.Keys) == 0 {
			return errors.Errorf("cache with path %q contains no keys", cache.Path)
		}

	}
	return nil
}

// VolumesAndMounts returns the volumes and mounts for the Caches
func (c Caches) VolumesAndMounts() (volumes []corev1.Volume, mounts []corev1.VolumeMount) {
	for _, cache := range c {
		volume, mount := cache.volumeAndMount()
		volumes = append(volumes, volume)
		mounts = append(mounts, mount)
	}
	return volumes, mounts
}

// Cache specifies a single cache entry.
type Cache struct {
	// Path is the overall path to be cached. Needs to be absolute, but can
	// contain environment variables.
	Path string `json:"path"`

	// Keys are a set of repository-relative paths which are used to identify
	// cache invalidation.
	Keys []string `json:"keys"`

	// DownloadOnly can be used to skip uploading the cache completely.
	DownloadOnly bool `json:"download_only"`
}

// Name returns a name for the cache which can be used for referencing
// purposes.
func (c *Cache) Name() string {
	const pathSep = "/"
	return strings.ReplaceAll(strings.Trim(c.Path, pathSep), pathSep, "-")
}

// TarBallName calculates the tarball name for the cache.
func (c *Cache) TarBallName() (string, error) {
	name := c.Name()
	for _, key := range c.Keys {
		f, err := os.Open(key)
		if err != nil {
			return "", errors.Wrapf(err, "open key file %q", key)
		}
		defer f.Close()

		hasher := sha256.New()
		if _, err := io.Copy(hasher, f); err != nil {
			return "", errors.Wrapf(err, "hashing key file %q", key)
		}
		name += "-" + hex.EncodeToString(hasher.Sum(nil))
	}
	return name + ".tar.gz", nil
}

func (c *Cache) volumeAndMount() (volume corev1.Volume, mount corev1.VolumeMount) {
	volume = corev1.Volume{
		Name: c.Name(),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	mount = corev1.VolumeMount{
		Name:      c.Name(),
		MountPath: c.Path,
	}
	return volume, mount
}
