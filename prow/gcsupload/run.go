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
	"os"
	"path"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// Run will upload files to GCS as prescribed by
// the options. Any extra files can be passed as
// a parameter and will have the prefix prepended
// to their destination in GCS, so the caller can
// operate relative to the base of the GCS dir.
func (o Options) Run(spec *downwardapi.JobSpec, extra map[string]gcs.UploadFunc) error {
	for extension, mediaType := range o.GCSConfiguration.MediaTypes {
		mime.AddExtensionType("."+extension, mediaType)
	}

	uploadTargets := o.assembleTargets(spec, extra)

	if !o.DryRun {
		ctx := context.Background()
		gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(o.GcsCredentialsFile))
		if err != nil {
			return fmt.Errorf("could not connect to GCS: %v", err)
		}

		if err := gcs.Upload(gcsClient.Bucket(o.Bucket), uploadTargets); err != nil {
			return fmt.Errorf("failed to upload to GCS: %v", err)
		}
	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	logrus.Info("Finished upload to GCS")
	return nil
}

func (o Options) assembleTargets(spec *downwardapi.JobSpec, extra map[string]gcs.UploadFunc) map[string]gcs.UploadFunc {
	jobBasePath, gcsPath, builder := PathsForJob(o.GCSConfiguration, spec, o.SubDir)

	uploadTargets := map[string]gcs.UploadFunc{}

	// ensure that an alias exists for any
	// job we're uploading artifacts for
	if alias := gcs.AliasForSpec(spec); alias != "" {
		fullBasePath := "gs://" + path.Join(o.Bucket, jobBasePath)
		uploadTargets[alias] = gcs.DataUploadWithMetadata(strings.NewReader(fullBasePath), map[string]string{
			"x-goog-meta-link": fullBasePath,
		})
	}

	if latestBuilds := gcs.LatestBuildForSpec(spec, builder); len(latestBuilds) > 0 {
		for _, latestBuild := range latestBuilds {
			dir, filename := path.Split(latestBuild)
			metadataFromFileName, attrs := gcs.AttributesFromFileName(filename)
			uploadTargets[path.Join(dir, metadataFromFileName)] = gcs.DataUploadWithAttributes(strings.NewReader(spec.BuildID), attrs)
		}
	}

	for _, item := range o.Items {
		info, err := os.Stat(item)
		if err != nil {
			logrus.Warnf("Encountered error in resolving items to upload for %s: %v", item, err)
			continue
		}
		if info.IsDir() {
			gatherArtifacts(item, gcsPath, info.Name(), uploadTargets)
		} else {
			metadataFromFileName, attrs := gcs.AttributesFromFileName(info.Name())
			destination := path.Join(gcsPath, metadataFromFileName)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				continue
			}
			uploadTargets[destination] = gcs.FileUploadWithAttributes(item, attrs)
		}
	}

	for destination, upload := range extra {
		uploadTargets[path.Join(gcsPath, destination)] = upload
	}

	return uploadTargets
}

// PathsForJob determines the following for a job:
//  - path in GCS under the bucket where job artifacts will be uploaded for:
//     - the job
//     - this specific run of the job (if any subdir is present)
// The builder for the job is also returned for use in other path resolution.
func PathsForJob(options *prowapi.GCSConfiguration, spec *downwardapi.JobSpec, subdir string) (string, string, gcs.RepoPathBuilder) {
	builder := builderForStrategy(options.PathStrategy, options.DefaultOrg, options.DefaultRepo)
	jobBasePath := gcs.PathForSpec(spec, builder)
	if options.PathPrefix != "" {
		jobBasePath = path.Join(options.PathPrefix, jobBasePath)
	}
	var gcsPath string
	if subdir == "" {
		gcsPath = jobBasePath
	} else {
		gcsPath = path.Join(jobBasePath, subdir)
	}

	return jobBasePath, gcsPath, builder
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

func gatherArtifacts(artifactDir, gcsPath, subDir string, uploadTargets map[string]gcs.UploadFunc) {
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
			dir, filename := path.Split(path.Join(gcsPath, subDir, relPath))
			metadataFromFileName, attrs := gcs.AttributesFromFileName(filename)
			destination := path.Join(dir, metadataFromFileName)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				return nil
			}
			logrus.Printf("Found %s in artifact directory. Uploading as %s\n", fspath, destination)
			uploadTargets[destination] = gcs.FileUploadWithAttributes(fspath, attrs)
		} else {
			logrus.Warnf("Encountered error in relative path calculation for %s under %s: %v", fspath, artifactDir, err)
		}
		return nil
	})
}
