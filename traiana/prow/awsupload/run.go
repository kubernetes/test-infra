package awsupload

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/traiana/prow/awsapi"
	"k8s.io/test-infra/traiana/prow/pod-utils/aws"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func Run(spec *downwardapi.JobSpec, dryRun bool, conf *kube.GCSConfiguration, items []string, subDir string) error {
	uploadTargets := assembleTargets(spec, conf, items, subDir)

	if !dryRun {
		session, err := awsapi.NewSession()

		if err != nil {
			return fmt.Errorf("could not connect to AWS: %v", err)
		}

		aws.Upload(awsapi.Bucket(conf.Bucket, session), uploadTargets)

	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	logrus.Info("Finished upload to AWS")
	return nil
}

func assembleTargets(spec *downwardapi.JobSpec, conf *kube.GCSConfiguration, items []string, subDir string) map[string]aws.UploadFunc {
	jobBasePath, gcsPath, builder := PathsForJob(conf, spec, subDir)

	uploadTargets := map[string]aws.UploadFunc{}

	// ensure that an alias exists for any
	// job we're uploading artifacts for
	if alias := gcs.AliasForSpec(spec); alias != "" {
		fullBasePath := "gs://" + path.Join(conf.Bucket, jobBasePath)
		uploadTargets[alias] = aws.DataUpload(strings.NewReader(fullBasePath))
	}

	if latestBuilds := gcs.LatestBuildForSpec(spec, builder); len(latestBuilds) > 0 {
		for _, latestBuild := range latestBuilds {
			uploadTargets[latestBuild] = aws.DataUpload(strings.NewReader(spec.BuildID))
		}
	}

	for _, item := range items {
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
			uploadTargets[destination] = aws.FileUpload(item)
		}
	}

	return uploadTargets
}

// PathsForJob determines the following for a job:
//  - path in AWS under the bucket where job artifacts will be uploaded for:
//     - the job
//     - this specific run of the job (if any subdir is present)
// The builder for the job is also returned for use in other path resolution.
func PathsForJob(conf *kube.GCSConfiguration, spec *downwardapi.JobSpec, subdir string) (string, string, gcs.RepoPathBuilder) {
	builder := builderForStrategy(conf.PathStrategy, conf.DefaultOrg, conf.DefaultRepo)
	jobBasePath := gcs.PathForSpec(spec, builder)
	if conf.PathPrefix != "" {
		jobBasePath = path.Join(conf.PathPrefix, jobBasePath)
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
	case kube.PathStrategyExplicit:
		builder = gcs.NewExplicitRepoPathBuilder()
	case kube.PathStrategyLegacy:
		builder = gcs.NewLegacyRepoPathBuilder(defaultOrg, defaultRepo)
	case kube.PathStrategySingle:
		builder = gcs.NewSingleDefaultRepoPathBuilder(defaultOrg, defaultRepo)
	}

	return builder
}

func gatherArtifacts(artifactDir, gcsPath, subDir string, uploadTargets map[string]aws.UploadFunc) {
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
			destination := path.Join(gcsPath, subDir, relPath)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				return nil
			}
			logrus.Printf("Found %s in artifact directory. Uploading as %s\n", fspath, destination)
			uploadTargets[destination] = aws.FileUpload(fspath)
		} else {
			logrus.Warnf("Encountered error in relative path calculation for %s under %s: %v", fspath, artifactDir, err)
		}
		return nil
	})
}