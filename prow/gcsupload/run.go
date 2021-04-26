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

package gcsupload

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// Run will upload files to GCS as prescribed by
// the options. Any extra files can be passed as
// a parameter and will have the prefix prepended
// to their destination in GCS, so the caller can
// operate relative to the base of the GCS dir.
func (o Options) Run(ctx context.Context, spec *downwardapi.JobSpec, extra map[string]gcs.UploadFunc) error {
	logrus.WithField("options", o).Debug("Uploading to blob storage")

	for extension, mediaType := range o.GCSConfiguration.MediaTypes {
		mime.AddExtensionType("."+extension, mediaType)
	}

	uploadTargets, err := o.assembleTargets(spec, extra)
	if err != nil {
		return fmt.Errorf("assembleTargets: %w", err)
	}

	if o.DryRun {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
		return nil
	}

	if o.LocalOutputDir == "" {
		if err := gcs.Upload(ctx, o.Bucket, o.StorageClientOptions.GCSCredentialsFile, o.StorageClientOptions.S3CredentialsFile, uploadTargets); err != nil {
			return fmt.Errorf("failed to upload to blob storage: %w", err)
		}
		logrus.Info("Finished upload to blob storage")
	} else {
		if err := gcs.LocalExport(ctx, o.LocalOutputDir, uploadTargets); err != nil {
			return fmt.Errorf("failed to copy files to %q: %w", o.LocalOutputDir, err)
		}
		logrus.Infof("Finished copying files to %q.", o.LocalOutputDir)
	}
	return nil
}

func (o Options) assembleTargets(spec *downwardapi.JobSpec, extra map[string]gcs.UploadFunc) (map[string]gcs.UploadFunc, error) {
	jobBasePath, blobStoragePath, builder := PathsForJob(o.GCSConfiguration, spec, o.SubDir)

	uploadTargets := map[string]gcs.UploadFunc{}

	// Skip the alias and latest build files in local mode.
	if o.LocalOutputDir == "" {
		// ensure that an alias exists for any
		// job we're uploading artifacts for
		if alias := gcs.AliasForSpec(spec); alias != "" {
			parsedBucket, err := url.Parse(o.Bucket)
			if err != nil {
				return nil, fmt.Errorf("parse bucket %q: %w", o.Bucket, err)
			}
			// only add gs:// prefix if o.Bucket itself doesn't already have a scheme prefix
			var fullBasePath string
			if parsedBucket.Scheme == "" {
				fullBasePath = "gs://" + path.Join(o.Bucket, jobBasePath)
			} else {
				fullBasePath = fmt.Sprintf("%s/%s", o.Bucket, jobBasePath)
			}
			uploadTargets[alias] = gcs.DataUploadWithMetadata(strings.NewReader(fullBasePath), map[string]string{
				"x-goog-meta-link": fullBasePath,
			})
		}

		if latestBuilds := gcs.LatestBuildForSpec(spec, builder); len(latestBuilds) > 0 {
			for _, latestBuild := range latestBuilds {
				dir, filename := path.Split(latestBuild)
				metadataFromFileName, writerOptions := gcs.WriterOptionsFromFileName(filename)
				uploadTargets[path.Join(dir, metadataFromFileName)] = gcs.DataUploadWithOptions(strings.NewReader(spec.BuildID), writerOptions)
			}
		}
	} else {
		// Remove the gcs path prefix in local mode so that items are rooted in the output dir without
		// excessive directory nesting.
		blobStoragePath = ""
	}

	for _, item := range o.Items {
		info, err := os.Stat(item)
		if err != nil {
			logrus.Warnf("Encountered error in resolving items to upload for %s: %v", item, err)
			continue
		}
		if info.IsDir() {
			gatherArtifacts(item, blobStoragePath, info.Name(), uploadTargets)
		} else {
			metadataFromFileName, writerOptions := gcs.WriterOptionsFromFileName(info.Name())
			destination := path.Join(blobStoragePath, metadataFromFileName)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				continue
			}
			uploadTargets[destination] = gcs.FileUploadWithOptions(item, writerOptions)
		}
	}

	for destination, upload := range extra {
		uploadTargets[path.Join(blobStoragePath, destination)] = upload
	}

	return uploadTargets, nil
}

// PathsForJob determines the following for a job:
//  - path in blob storage under the bucket where job artifacts will be uploaded for:
//     - the job
//     - this specific run of the job (if any subdir is present)
// The builder for the job is also returned for use in other path resolution.
func PathsForJob(options *prowapi.GCSConfiguration, spec *downwardapi.JobSpec, subdir string) (string, string, gcs.RepoPathBuilder) {
	builder := builderForStrategy(options.PathStrategy, options.DefaultOrg, options.DefaultRepo)
	jobBasePath := gcs.PathForSpec(spec, builder)
	if options.PathPrefix != "" {
		jobBasePath = path.Join(options.PathPrefix, jobBasePath)
	}
	var blobStoragePath string
	if subdir == "" {
		blobStoragePath = jobBasePath
	} else {
		blobStoragePath = path.Join(jobBasePath, subdir)
	}

	return jobBasePath, blobStoragePath, builder
}

func builderForStrategy(strategy, defaultOrg, defaultRepo string) gcs.RepoPathBuilder {
	var builder gcs.RepoPathBuilder
	switch strategy {
	case prowapi.PathStrategyExplicit:
		builder = gcs.NewExplicitRepoPathBuilder()
	case prowapi.PathStrategyLegacy:
		builder = gcs.NewLegacyRepoPathBuilder(defaultOrg, defaultRepo)
	case prowapi.PathStrategySingle:
		builder = gcs.NewSingleDefaultRepoPathBuilder(defaultOrg, defaultRepo)
	}

	return builder
}

func gatherArtifacts(artifactDir, blobStoragePath, subDir string, uploadTargets map[string]gcs.UploadFunc) {
	logrus.Printf("Gathering artifacts from artifact directory: %s", artifactDir)
	filepath.Walk(artifactDir, func(fspath string, info os.FileInfo, err error) error {
		if info == nil || info.IsDir() {
			return nil
		}

		// we know path will be below artifactDir, but we can't
		// communicate that to the filepath module. We can ignore
		// this error as we can be certain it won't occur and best-
		// effort upload is OK in any case
		if relPath, err := filepath.Rel(artifactDir, fspath); err == nil {
			dir, filename := path.Split(path.Join(blobStoragePath, subDir, relPath))
			metadataFromFileName, writerOptions := gcs.WriterOptionsFromFileName(filename)
			destination := path.Join(dir, metadataFromFileName)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				return nil
			}
			logrus.Printf("Found %s in artifact directory. Uploading as %s\n", fspath, destination)
			uploadTargets[destination] = gcs.FileUploadWithOptions(fspath, writerOptions)
		} else {
			logrus.Warnf("Encountered error in relative path calculation for %s under %s: %v", fspath, artifactDir, err)
		}
		return nil
	})
}
