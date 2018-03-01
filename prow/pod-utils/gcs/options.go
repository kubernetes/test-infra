/*
Copyright 2017 The Kubernetes Authors.

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

package gcs

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	"k8s.io/test-infra/prow/pjutil"
)

type pathStrategyType string

const (
	pathStrategyLegacy   pathStrategyType = "legacy"
	pathStrategyExplicit                  = "explicit"
	pathStrategySingle                    = "single"
)

// BindOptions adds flags to the FlagSet that populate
// the GCS upload options struct returned.
func BindOptions(fs *flag.FlagSet) *Options {
	o := Options{}
	fs.StringVar(&o.SubDir, "sub-dir", "", "Optional sub-directory of the job's path to which artifacts are uploaded")

	fs.StringVar(&o.PathStrategy, "path-strategy", pathStrategyExplicit, "how to encode org and repo into GCS paths")
	fs.StringVar(&o.DefaultOrg, "default-org", "", "optional default org for GCS path encoding")
	fs.StringVar(&o.DefaultRepo, "default-repo", "", "optional default repo for GCS path encoding")

	fs.StringVar(&o.GcsBucket, "gcs-bucket", "", "GCS bucket to upload into")
	fs.StringVar(&o.GceCredentialsFile, "gcs-credentials-file", "", "file where Google Cloud authentication credentials are stored")
	fs.BoolVar(&o.DryRun, "dry-run", true, "do not interact with GCS")
	return &o
}

// Options exposes the configuration necessary
// for defining where in GCS an upload will land.
type Options struct {
	// Items are files or directories to upload
	Items []string

	// SubDir is appended to the resolved GCS path
	SubDir string

	// PathStrategy and the default Org and Repo
	// determine how the GCS path is resolved from
	// Prow-specified environment variables
	PathStrategy string
	DefaultOrg   string
	DefaultRepo  string

	GcsBucket          string
	GceCredentialsFile string
	DryRun             bool
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.Items = args
}

// Validate ensures that the set of options are
// self-consistent and valid
func (o *Options) Validate() error {
	if !o.DryRun {
		if o.GcsBucket == "" {
			return errors.New("GCS upload was requested but required flag --gcs-bucket was unset")
		}

		if o.GceCredentialsFile == "" {
			return errors.New("GCS upload was requested but required flag --gcs-credentials-file was unset")
		}
	}

	strategy := pathStrategyType(o.PathStrategy)
	if strategy != pathStrategyLegacy && strategy != pathStrategyExplicit && strategy != pathStrategySingle {
		return fmt.Errorf("GCS path strategy must be one of %q, %q, or %q", pathStrategyLegacy, pathStrategyExplicit, pathStrategySingle)
	}

	if strategy != pathStrategyExplicit && (o.DefaultOrg == "" || o.DefaultRepo == "") {
		return fmt.Errorf("default org and repo must be provided for GCS strategy %q", strategy)
	}

	return nil
}

// Run will upload files to GCS as prescribed by
// the options. Any extra files can be passed as
// a parameter and will have the prefix prepended
// to their destination in GCS, so the caller can
// operate relative to the base of the GCS dir.
func (o *Options) Run(extra map[string]UploadFunc) error {
	var builder RepoPathBuilder
	switch pathStrategyType(o.PathStrategy) {
	case pathStrategyExplicit:
		builder = NewExplicitRepoPathBuilder()
	case pathStrategyLegacy:
		builder = NewLegacyRepoPathBuilder(o.DefaultOrg, o.DefaultRepo)
	case pathStrategySingle:
		builder = NewSingleDefaultRepoPathBuilder(o.DefaultOrg, o.DefaultRepo)
	}

	spec, err := pjutil.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	var gcsPath string
	jobBasePath := PathForSpec(spec, builder)
	if o.SubDir == "" {
		gcsPath = jobBasePath
	} else {
		gcsPath = path.Join(jobBasePath, o.SubDir)
	}

	uploadTargets := map[string]UploadFunc{}

	// ensure that an alias exists for any
	// job we're uploading artifacts for
	if alias := AliasForSpec(spec); alias != "" {
		uploadTargets[alias] = DataUpload(strings.NewReader(jobBasePath))
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
			destination := path.Join(gcsPath, info.Name())
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				continue
			}
			uploadTargets[destination] = FileUpload(item)
		}
	}

	for destination, upload := range extra {
		uploadTargets[path.Join(gcsPath, destination)] = upload
	}

	if !o.DryRun {
		ctx := context.Background()
		gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(o.GceCredentialsFile))
		if err != nil {
			return fmt.Errorf("could not connect to GCS: %v", err)
		}

		if err := Upload(gcsClient.Bucket(o.GcsBucket), uploadTargets); err != nil {
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

func gatherArtifacts(artifactDir, gcsPath, subDir string, uploadTargets map[string]UploadFunc) {
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
			destination := path.Join(subDir, relPath)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				return nil
			}
			logrus.Printf("Found %s in artifact directory. Uploading as %s\n", fspath, destination)
			uploadTargets[path.Join(gcsPath, destination)] = FileUpload(fspath)
		} else {
			logrus.Warnf("Encountered error in relative path calculation for %s under %s: %v", fspath, artifactDir, err)
		}
		return nil
	})
}
