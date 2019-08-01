package qiniu

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// NewOptions returns an empty Options with no nil fields.
func NewOptions() *Options {
	return &Options{
		GCSConfiguration: &prowapi.GCSConfiguration{},
	}
}

// Options exposes the configuration necessary
// for defining where in GCS an upload will land.
type Options struct {
	*prowapi.GCSConfiguration

	// Items are files or directories to upload.
	Items  []string `json:"items,omitempty"`
	DryRun bool     `json:"dry_run"`

	// TODO(CarlJi):理论上这些配置应该放到CRD里，这样全局就可以传递
	// 但考虑到操作CRD，风险较高，这里为了简化，希望使用者外部直接传入这些信息
	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

// Validate ensures that the set of options are
// self-consistent and valid.
func (o *Options) Validate() error {
	if !o.DryRun {
		if o.Bucket == "" {
			return errors.New("no Qiniu bucket was provided")
		}

		if o.AccessKey == "" || o.SecretKey == "" {
			return errors.New("no Qiniu secret was provided")
		}
	}

	return nil
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.Items = args
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable.
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}

// AddFlags adds flags to the FlagSet that populate
// the GCS upload options struct given.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.Bucket, "bucket", "", "bucket where the artifacts uploaded to")
	fs.StringVar(&o.AccessKey, "access-key", "", "key to access qiniu bucket")
	fs.StringVar(&o.SecretKey, "secret-key", "", "secret to access qiniu bucket")

	fs.BoolVar(&o.DryRun, "dry-run", true, "do not interact with cloud")
}

// Run will upload files to qiniu as prescribed by
// the options. Any extra files can be passed as
// a parameter and will have the prefix prepended
// to their destination in qiniu, so the caller can
// operate relative to the base of the qiniu dir.
func (o Options) Run(spec *downwardapi.JobSpec, extra map[string]UploadFunc) error {
	uploadTargets := o.assembleTargets(spec, extra)

	if !o.DryRun {
		qn, err := NewUploader(o.Bucket, o.AccessKey, o.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to init qiniu uploader: %v", err)
		}

		if err := qn.Upload(uploadTargets); err != nil {
			return fmt.Errorf("failed to upload to Qiniu: %v", err)
		}
	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	return nil
}

func (o Options) assembleTargets(spec *downwardapi.JobSpec, extra map[string]UploadFunc) map[string]UploadFunc {
	_, gcsPath, builder := gcsupload.PathsForJob(o.GCSConfiguration, spec, "")
	uploadTargets := map[string]UploadFunc{}

	if latestBuilds := gcs.LatestBuildForSpec(spec, builder); len(latestBuilds) > 0 {
		for _, latestBuild := range latestBuilds {
			uploadTargets[latestBuild] = DataUpload(strings.NewReader(spec.BuildID))
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

	return uploadTargets
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
			destination := path.Join(gcsPath, subDir, relPath)
			if _, exists := uploadTargets[destination]; exists {
				logrus.Warnf("Encountered duplicate upload of %s, skipping...", destination)
				return nil
			}
			logrus.Printf("Found %s in artifact directory. Uploading as %s\n", fspath, destination)
			uploadTargets[destination] = FileUpload(fspath)
		} else {
			logrus.Warnf("Encountered error in relative path calculation for %s under %s: %v", fspath, artifactDir, err)
		}
		return nil
	})
}
