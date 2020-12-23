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

package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/util"
	"k8s.io/test-infra/prow/io"
)

const reporterName = "gcsreporter"

type gcsReporter struct {
	cfg    config.Getter
	dryRun bool
	author util.Author
}

func (gr *gcsReporter) Report(log *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // TODO: pass through a global context?
	defer cancel()

	_, _, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		log.WithError(err).Info("Not uploading prowjob because we couldn't find a destination")
		return []*prowv1.ProwJob{pj}, nil, nil
	}
	stateErr := gr.reportJobState(ctx, log, pj)
	prowjobErr := gr.reportProwjob(ctx, log, pj)

	return []*prowv1.ProwJob{pj}, nil, utilerrors.NewAggregate([]error{stateErr, prowjobErr})
}

func (gr *gcsReporter) reportJobState(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	startedErr := gr.reportStartedJob(ctx, log, pj)
	var finishedErr error
	if pj.Complete() {
		finishedErr = gr.reportFinishedJob(ctx, log, pj)
	}
	return utilerrors.NewAggregate([]error{startedErr, finishedErr})
}

// reportStartedJob uploads a started.json for the job. This will almost certainly
// happen before the pod itself gets to upload one, at which point the pod will
// upload its own. If for some reason one already exists, it will not be overwritten.
func (gr *gcsReporter) reportStartedJob(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	s := metadata.Started{
		Timestamp: pj.Status.StartTime.Unix(),
		Metadata:  metadata.Metadata{"uploader": "crier"},
	}
	output, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal started metadata: %v", err)
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload started.json")
		return nil
	}
	return util.WriteContent(ctx, log, gr.author, bucketName, path.Join(dir, prowv1.StartedStatusFile), false, output)
}

// reportFinishedJob uploads a finished.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportFinishedJob(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	if !pj.Complete() {
		return errors.New("cannot report finished.json for incomplete job")
	}
	completion := pj.Status.CompletionTime.Unix()
	passed := pj.Status.State == prowv1.SuccessState
	f := metadata.Finished{
		Timestamp: &completion,
		Passed:    &passed,
		Metadata:  metadata.Metadata{"uploader": "crier"},
		Result:    string(pj.Status.State),
	}
	output, err := json.MarshalIndent(f, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal finished metadata: %v", err)
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload finished.json")
		return nil
	}
	return util.WriteContent(ctx, log, gr.author, bucketName, path.Join(dir, prowv1.FinishedStatusFile), false, output)
}

func (gr *gcsReporter) reportProwjob(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	// Unconditionally dump the prowjob to GCS, on all job updates.
	output, err := json.MarshalIndent(pj, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal prowjob: %v", err)
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload pod info")
		return nil
	}
	return util.WriteContent(ctx, log, gr.author, bucketName, path.Join(dir, "prowjob.json"), true, output)
}

func (gr *gcsReporter) GetName() string {
	return reporterName
}

func (gr *gcsReporter) ShouldReport(_ *logrus.Entry, pj *prowv1.ProwJob) bool {
	// We can only report jobs once they have a build ID. By denying responsibility
	// for it until it has one, crier will not mark us as having handled it until
	// it is possible for us to handle it, ensuring that we get a chance to see it.
	return pj.Status.BuildID != ""
}

func New(cfg config.Getter, opener io.Opener, dryRun bool) *gcsReporter {
	return newWithAuthor(cfg, util.StorageAuthor{Opener: opener}, dryRun)
}

func newWithAuthor(cfg config.Getter, author util.Author, dryRun bool) *gcsReporter {
	return &gcsReporter{
		cfg:    cfg,
		dryRun: dryRun,
		author: author,
	}
}
