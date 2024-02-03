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
	"fmt"
	"path"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/util"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const reporterName = "gcsreporter"

type gcsReporter struct {
	cfg    config.Getter
	dryRun bool
	opener io.Opener
}

func (gr *gcsReporter) Report(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
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
	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %w", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload started.json")
		return nil
	}

	// Best-effort read of existing started.json; it's overwritten only if it's uploaded
	// by crier and there is something new (clone record).
	var existingStarted metadata.Started
	var existing bool
	startedFilePath, err := providers.StoragePath(bucketName, path.Join(dir, prowv1.StartedStatusFile))
	if err != nil {
		// Started.json storage path is invalid, so this function will
		// eventually fail.
		return fmt.Errorf("failed to resolve started.json path: %v", err)
	}

	content, err := io.ReadContent(ctx, log, gr.opener, startedFilePath)
	if err != nil {
		if !io.IsNotExist(err) {
			log.WithError(err).Warn("Failed to read started.json.")
		}
	} else {
		err = json.Unmarshal(content, &existingStarted)
		if err != nil {
			log.WithError(err).Warn("Failed to unmarshal started.json.")
		} else {
			existing = true
		}
	}

	if existing && (existingStarted.Metadata == nil || existingStarted.Metadata["uploader"] != "crier") {
		// Uploaded by prowjob itself, skip reporting
		log.Debug("Uploaded by pod-utils, skipping")
		return nil
	}

	staticRevision := downwardapi.GetRevisionFromRefs(pj.Spec.Refs, pj.Spec.ExtraRefs)
	if pj.Spec.Refs == nil || (existingStarted.RepoCommit != "" && existingStarted.RepoCommit != staticRevision) {
		// RepoCommit could only be "", BaseRef, or the final resolved SHA,
		// which shouldn't change for a given presubmit job. Avoid query GCS is
		// this is already done.
		log.Debug("RepoCommit already resolved before, skipping")
		return nil
	}

	// Try to read clone records
	cloneRecord := make([]clone.Record, 0)
	cloneRecordFilePath, err := providers.StoragePath(bucketName, path.Join(dir, prowv1.CloneRecordFile))
	if err != nil {
		// This is user config error
		log.WithError(err).Debug("Failed to resolve clone-records.json path.")
	} else {
		cloneRecordBytes, err := io.ReadContent(ctx, log, gr.opener, cloneRecordFilePath)
		if err != nil {
			if !io.IsNotExist(err) {
				log.WithError(err).Warn("Failed to read clone records.")
			}
		} else {
			if err := json.Unmarshal(cloneRecordBytes, &cloneRecord); err != nil {
				log.WithError(err).Warn("Failed to unmarshal clone records.")
			}
		}
	}
	s := downwardapi.PjToStarted(pj, cloneRecord)
	s.Metadata = metadata.Metadata{"uploader": "crier"}

	output, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal started metadata: %w", err)
	}

	// Overwrite if it was uploaded by crier(existing) and there might be
	// something new.
	// Add a new var for better readability.
	overwrite := existing
	overwriteOpt := io.WriterOptions{PreconditionDoesNotExist: utilpointer.Bool(!overwrite)}
	return io.WriteContent(ctx, log, gr.opener, startedFilePath, output, overwriteOpt)
}

// reportFinishedJob uploads a finished.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportFinishedJob(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	output, err := util.MarshalFinishedJSON(pj)
	if err != nil {
		return fmt.Errorf("failed to marshal finished metadata: %w", err)
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %w", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload finished.json")
		return nil
	}
	//PreconditionDoesNotExist:true means create only when file not exist.
	overwriteOpt := io.WriterOptions{PreconditionDoesNotExist: utilpointer.Bool(true)}
	finishedFilePath, err := providers.StoragePath(bucketName, path.Join(dir, prowv1.FinishedStatusFile))
	if err != nil {
		return fmt.Errorf("failed to resolve finished.json path: %v", err)
	}
	return io.WriteContent(ctx, log, gr.opener, finishedFilePath, output, overwriteOpt)
}

func (gr *gcsReporter) reportProwjob(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	// Unconditionally dump the ProwJob to GCS, on all job updates.
	output, err := util.MarshalProwJob(pj)
	if err != nil {
		return fmt.Errorf("failed to marshal ProwJob: %w", err)
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %w", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Debug("Would upload pod info")
		return nil
	}
	overWriteOpts := io.WriterOptions{PreconditionDoesNotExist: utilpointer.Bool(false)}
	prowJobFilePath, err := providers.StoragePath(bucketName, path.Join(dir, prowv1.ProwJobFile))
	if err != nil {
		return fmt.Errorf("failed to resolve prowjob.json path: %v", err)
	}
	return io.WriteContent(ctx, log, gr.opener, prowJobFilePath, output, overWriteOpts)
}

func (gr *gcsReporter) GetName() string {
	return reporterName
}

func (gr *gcsReporter) ShouldReport(_ context.Context, _ *logrus.Entry, pj *prowv1.ProwJob) bool {
	// We can only report jobs once they have a build ID. By denying responsibility
	// for it until it has one, crier will not mark us as having handled it until
	// it is possible for us to handle it, ensuring that we get a chance to see it.
	return pj.Status.BuildID != ""
}

func New(cfg config.Getter, opener io.Opener, dryRun bool) *gcsReporter {
	return &gcsReporter{
		cfg:    cfg,
		dryRun: dryRun,
		opener: opener,
	}
}
