/*
Copyright 2023 The Kubernetes Authors.

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

package resultstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/util"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/resultstore"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reporter reports Prow results to ResultStore and satisfies the
// crier.reportClient interface.
type Reporter struct {
	cfg      config.Getter
	opener   io.Opener
	uploader *resultstore.Uploader
	dirOnly  bool
}

// New returns a new Reporter.
func New(cfg config.Getter, opener io.Opener, uploader *resultstore.Uploader, dirOnly bool) *Reporter {
	return &Reporter{
		cfg:      cfg,
		opener:   opener,
		uploader: uploader,
		dirOnly:  dirOnly,
	}
}

// GetName returns the name of this reporter.
func (r *Reporter) GetName() string {
	return "resultstorereporter"
}

// ShouldReport returns whether results should be reported for this
// job at this time.
func (r *Reporter) ShouldReport(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) bool {
	if !pj.Spec.Report {
		return false
	}

	// Require configured resultstore ProjectID for now. It may be determined
	// automatically from storage in the future.
	if projectID(pj) == "" {
		return false
	}

	// ResultStore requires files stored in GCS.
	if !util.IsGCSDestination(r.cfg, pj) {
		return false
	}

	if pj.Status.State == v1.TriggeredState || pj.Status.State == v1.PendingState {
		log.Info("job not finished")
		return false
	}

	return true
}

func projectID(pj *v1.ProwJob) string {
	if pj.Spec.ReporterConfig == nil || pj.Spec.ReporterConfig.ResultStore == nil {
		return ""
	}
	return pj.Spec.ReporterConfig.ResultStore.ProjectID
}

// Report reports results for this job to ResultStore.
func (r *Reporter) Report(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	bucket, dir, err := util.GetJobDestination(r.cfg, pj)
	if err != nil {
		return nil, nil, err
	}
	path, err := providers.StoragePath(bucket, dir)
	if err != nil {
		return nil, nil, err
	}
	log = log.WithField("BuildID", pj.Status.BuildID)
	finished := readFinishedFile(ctx, log, r.opener, path)
	started := readStartedFile(ctx, log, r.opener, path)
	files, err := resultstore.ArtifactFiles(ctx, r.opener, resultstore.ArtifactOpts{Dir: path, ArtifactsDirOnly: r.dirOnly})
	if err != nil {
		// Log and continue in case of errors.
		log.WithError(err).Errorf("error reading artifact files from %q", path)
	}
	err = r.uploader.Upload(ctx, log, &resultstore.Payload{
		Job:       pj,
		Started:   started,
		Finished:  finished,
		Files:     files,
		ProjectID: projectID(pj),
	})
	if err != nil {
		log.WithError(err).Error("resultstore upload error")
	}
	return []*v1.ProwJob{pj}, nil, err
}

// There is a race between this and the GCS reporter in the case the
// finished.json file does not exist. This reporter waits for it to
// be written by the GCS reporter so it is not missed.

var waitFinishedBackoff = func() wait.Backoff {
	return wait.Backoff{
		Duration: time.Second,
		Factor:   2,
		Cap:      15 * time.Second,
	}
}

func readFinishedFile(ctx context.Context, log *logrus.Entry, opener io.Opener, dir string) *metadata.Finished {
	n := dir + "/" + v1.FinishedStatusFile
	var finished *metadata.Finished
	err := wait.ExponentialBackoffWithContext(ctx, waitFinishedBackoff(), func() (bool, error) {
		bs, err := io.ReadContent(ctx, log, opener, n)
		if err != nil {
			if !io.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		if err := json.Unmarshal(bs, &finished); err != nil {
			return false, fmt.Errorf("unmarshal: %w", err)
		}
		return true, nil
	})
	if err != nil {
		log.WithError(err).Errorf("Failed to read %q", n)
	}
	return finished
}

func readStartedFile(ctx context.Context, log *logrus.Entry, opener io.Opener, dir string) *metadata.Started {
	n := dir + "/" + v1.StartedStatusFile
	bs, err := io.ReadContent(ctx, log, opener, n)
	if err != nil {
		if !io.IsNotExist(err) {
			log.WithError(err).Warnf("Failed to read %q", v1.StartedStatusFile)
		}
		return nil
	}
	var started metadata.Started
	if err := json.Unmarshal(bs, &started); err != nil {
		log.WithError(err).Warnf("Failed to unmarshal %q", n)
		return nil
	}
	return &started
}
