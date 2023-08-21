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
	"fmt"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/util"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reporter reports Prow results to ResultStore and satisfies the
// crier.reportClient interface.
type Reporter struct {
	cfg    config.Getter
	opener io.Opener
}

// New returns a new Reporter.
func New(cfg config.Getter, opener io.Opener) *Reporter {
	return &Reporter{
		cfg:    cfg,
		opener: opener,
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
	_, err = providers.StoragePath(bucket, dir)
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, fmt.Errorf("not yet implemented")
}
