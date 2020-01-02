/*
Copyright 2019 The Kubernetes Authors.

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

package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
	"k8s.io/test-infra/prow/errorutil"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const reporterName = "gcsreporter"

type gcsReporter struct {
	cfg     config.Getter
	dryRun  bool
	logger  *logrus.Entry
	storage *storage.Client
}

func (gr *gcsReporter) Report(pj *prowv1.ProwJob) ([]*prowv1.ProwJob, error) {
	ctx := context.Background() // TODO: get this from somewhere better?

	_, _, err := gr.getJobDestination(pj)
	if err != nil {
		gr.logger.Infof("Not uploading %q (%s#%s) because we couldn't find a destination.", pj.Name, pj.Spec.Job, pj.Status.BuildID)
		return []*prowv1.ProwJob{pj}, nil
	}
	stateErr := gr.reportJobState(ctx, pj)
	prowjobErr := gr.reportProwjob(ctx, pj)

	return []*prowv1.ProwJob{pj}, errorutil.NewAggregate(stateErr, prowjobErr)
}

func (gr *gcsReporter) reportJobState(ctx context.Context, pj *prowv1.ProwJob) error {
	if pj.Status.State == prowv1.TriggeredState {
		return gr.reportStartedJob(ctx, pj)
	}
	if pj.Complete() {
		return gr.reportFinishedJob(ctx, pj)
	}
	return nil
}

// reportStartedJob uploads a started.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportStartedJob(ctx context.Context, pj *prowv1.ProwJob) error {
	// We only upload started.json when we first start.
	if pj.Status.State != prowv1.TriggeredState {
		return nil
	}

	f := metadata.Started{
		Timestamp: pj.Status.StartTime.Unix(),
		Metadata:  metadata.Metadata{"uploader": "crier"},
	}
	output, err := json.Marshal(f)
	if err != nil {
		return err
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return err
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload started.json to %q/%q", bucketName, dir)
		return nil
	}
	b := gr.storage.Bucket(bucketName)
	w := b.Object(path.Join(dir, "started.json")).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	return gr.writeContent(w, output)
}

// reportFinishedJob uploads a finished.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportFinishedJob(ctx context.Context, pj *prowv1.ProwJob) error {
	// We don't upload a finished.json until we finish.
	if !pj.Complete() {
		return nil
	}

	completion := pj.Status.CompletionTime.Unix()
	passed := pj.Status.State == prowv1.SuccessState
	f := metadata.Finished{
		Timestamp: &completion,
		Passed:    &passed,
		Metadata:  metadata.Metadata{"uploader": "crier"},
		Result:    string(pj.Status.State),
	}
	output, err := json.Marshal(f)
	if err != nil {
		return err
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return err
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload finished.json info to %q/%q", bucketName, dir)
		return nil
	}
	b := gr.storage.Bucket(bucketName)
	w := b.Object(path.Join(dir, "finished.json")).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	return gr.writeContent(w, output)
}

func (gr *gcsReporter) reportProwjob(ctx context.Context, pj *prowv1.ProwJob) error {
	// Unconditionally dump the prowjob to GCS, on all job updates.
	output, err := json.Marshal(pj)
	if err != nil {
		return err
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return err
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload pod info to %q/%q", bucketName, dir)
		return nil
	}
	b := gr.storage.Bucket(bucketName)
	w := b.Object(path.Join(dir, "prowjob.json")).NewWriter(ctx)
	return gr.writeContent(w, output)
}

func (gr *gcsReporter) writeContent(w *storage.Writer, content []byte) error {
	_, err := w.Write(content)
	var reportErr error
	if isErrUnexpected(err) {
		reportErr = err
		gr.logger.WithError(err).WithFields(logrus.Fields{"bucket": w.Bucket, "name": w.Name}).Warn("Uploading info to GCS failed (write)")
	}
	err = w.Close()
	if isErrUnexpected(err) {
		reportErr = err
		gr.logger.WithError(err).WithFields(logrus.Fields{"bucket": w.Bucket, "name": w.Name}).Warn("Uploading info to GCS failed (close)")
	}
	return reportErr
}

func isErrUnexpected(err error) bool {
	if err == nil {
		return false
	}
	// Precondition Failed is expected and we can silently ignore it.
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == http.StatusPreconditionFailed {
			return false
		}
		return true
	}
	return true
}

func (gr *gcsReporter) getJobDestination(pj *prowv1.ProwJob) (bucket string, dir string, err error) {
	// The decoration config is always provided for decorated jobs, but many
	// jobs are not decorated, so we guess that we should use the default location
	// for those jobs. This assumption is usually (but not always) correct.
	// The TestGrid configurator uses the same assumption.
	ddc := gr.cfg().Plank.GetDefaultDecorationConfigs(pj.Spec.Refs.Repo)

	var gcsConfig *prowv1.GCSConfiguration
	if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.GCSConfiguration != nil {
		gcsConfig = pj.Spec.DecorationConfig.GCSConfiguration
	} else if ddc != nil && ddc.GCSConfiguration != nil {
		gcsConfig = ddc.GCSConfiguration
	} else {
		return "", "", fmt.Errorf("couldn't figure out a GCS config for %q", pj.Spec.Job)
	}

	_, dir, _ = gcsupload.PathsForJob(gcsConfig, &downwardapi.JobSpec{
		Type:      pj.Spec.Type,
		Job:       pj.Spec.Job,
		BuildID:   pj.Status.BuildID,
		ProwJobID: pj.Name,
		Refs:      pj.Spec.Refs,
		ExtraRefs: pj.Spec.ExtraRefs,
	}, "")

	return gcsConfig.Bucket, dir, nil
}

func (gr *gcsReporter) GetName() string {
	return reporterName
}

func (gr *gcsReporter) ShouldReport(pj *prowv1.ProwJob) bool {
	return true
}

func New(cfg config.Getter, storage *storage.Client, dryRun bool) *gcsReporter {
	return &gcsReporter{
		cfg:     cfg,
		dryRun:  dryRun,
		logger:  logrus.WithField("component", reporterName),
		storage: storage,
	}
}
