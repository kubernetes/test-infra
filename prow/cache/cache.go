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

package cache

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"path"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// CopyFromGCS calculates the key paths from the source root and retrieves the
// caches from GCS if existing locally.
func CopyFromGCS(caches v1.Caches, srcRoot string, gcsOptions *gcsupload.Options) (err error) {
	if caches == nil {
		// no caches provided, nothing to do
		return nil
	}
	if gcsOptions == nil {
		return errors.New("no GCS options provided but cache specified")
	}
	switchBack, err := changeDir(srcRoot)
	if err != nil {
		return errors.Wrap(err, "change into source root")
	}
	defer func() { err = switchBack(err) }()

	// TODO: download is unimplemented

	return nil
}

// CopyToGCS takes the provided caches and uploads them to GCS. It assumes that
// all cache paths are available via mounts.
func CopyToGCS(caches v1.Caches, srcRoot string, gcsOptions *gcsupload.Options) (err error) {
	if caches == nil {
		// no caches provided, nothing to do
		logrus.Debug("Nothing to cache since no cache provided")
		return nil
	}
	if gcsOptions == nil {
		return errors.New("no GCS options provided but cache specified")
	}
	switchBack, err := changeDir(srcRoot)
	if err != nil {
		return errors.Wrap(err, "change into source root")
	}
	defer func() { err = switchBack(err) }()

	logrus.Infof("Uploading %d caches", len(caches))

	// Build the upload targets
	uploadTargets := map[string]gcs.UploadFunc{}
	for _, cache := range caches {
		if cache.DownloadOnly {
			logrus.Infof("Skipping download only cache %s", cache.Name())
			continue
		}

		// Build the remote path
		basePath := cacheRemoteBasePath(gcsOptions)
		tarballPath, err := cache.TarBallName()
		if err != nil {
			return errors.Wrapf(
				err, "build key tarball name for cache %q", cache.Name(),
			)
		}
		remotePath := path.Join(basePath, tarballPath)

		// TODO: createa the tarball

		uploadTargets[remotePath] = gcs.FileUpload(cache.Path)
	}

	// TODO: do not fail if cache already exists remotely
	if err := gcs.Upload(
		gcsOptions.Bucket,
		gcsOptions.StorageClientOptions.GCSCredentialsFile,
		gcsOptions.StorageClientOptions.S3CredentialsFile,
		uploadTargets,
	); err != nil {
		return errors.Wrap(err, "upload cache to blob storage")
	}

	return nil
}

// cacheRemoteBasePath returns the remote bucket base path for storing the
// caches
func cacheRemoteBasePath(gcsOptions *gcsupload.Options) string {
	return path.Join(gcsOptions.PathPrefix, "cache")
}

// changeDir is a helper to change to a dir and switch back after doing an
// operation
func changeDir(to string) (changeBackFn func(error) error, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "get current working dir")
	}
	if err := os.Chdir(to); err != nil {
		return nil, errors.Wrapf(err, "switch into dir %q", to)
	}
	return func(err error) error {
		if chDirErr := os.Chdir(cwd); err != nil {
			err = errors.Wrapf(err, "switch into previous working dir %q: %v", cwd, chDirErr)
		}
		return err
	}, nil
}
