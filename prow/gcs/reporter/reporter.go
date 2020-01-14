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

package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

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
	cfg    config.Getter
	dryRun bool
	logger *logrus.Entry
	author author
}

type author interface {
	NewWriter(ctx context.Context, bucket, path string, overwrite bool) io.WriteCloser
}

type storageAuthor struct {
	client *storage.Client
}

func (sa storageAuthor) NewWriter(ctx context.Context, bucket, path string, overwrite bool) io.WriteCloser {
	obj := sa.client.Bucket(bucket).Object(path)
	if !overwrite {
		obj = obj.If(storage.Conditions{DoesNotExist: true})
	}
	return obj.NewWriter(ctx)
}

func (gr *gcsReporter) Report(pj *prowv1.ProwJob) ([]*prowv1.ProwJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // TODO: pass through a global context?
	defer cancel()

	_, _, err := gr.getJobDestination(pj)
	if err != nil {
		gr.logger.Infof("Not uploading %q (%s#%s) because we couldn't find a destination: %v", pj.Name, pj.Spec.Job, pj.Status.BuildID, err)
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

// reportStartedJob uploads a started.json for the job. This will almost certainly
// happen before the pod itself gets to upload one, at which point the pod will
// upload its own. If for some reason one already exists, it will not be overwritten.
func (gr *gcsReporter) reportStartedJob(ctx context.Context, pj *prowv1.ProwJob) error {
	s := metadata.Started{
		Timestamp: pj.Status.StartTime.Unix(),
		Metadata:  metadata.Metadata{"uploader": "crier"},
	}
	output, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal started metadata: %v", err)
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload started.json to %q/%q", bucketName, dir)
		return nil
	}
	return gr.writeContent(ctx, bucketName, path.Join(dir, "started.json"), false, output)
}

// reportFinishedJob uploads a finished.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportFinishedJob(ctx context.Context, pj *prowv1.ProwJob) error {
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
		return fmt.Errorf("failed to marshal finished metadata: %v", err)
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload finished.json info to %q/%q", bucketName, dir)
		return nil
	}
	return gr.writeContent(ctx, bucketName, path.Join(dir, "finished.json"), false, output)
}

func (gr *gcsReporter) reportProwjob(ctx context.Context, pj *prowv1.ProwJob) error {
	// Unconditionally dump the prowjob to GCS, on all job updates.
	output, err := json.Marshal(pj)
	if err != nil {
		return fmt.Errorf("failed to marshal prowjob: %v", err)
	}

	bucketName, dir, err := gr.getJobDestination(pj)
	if err != nil {
		return fmt.Errorf("failed to get job destination: %v", err)
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload pod info to %q/%q", bucketName, dir)
		return nil
	}
	return gr.writeContent(ctx, bucketName, path.Join(dir, "prowjob.json"), true, output)
}

func (gr *gcsReporter) writeContent(ctx context.Context, bucket, path string, overwrite bool, content []byte) error {
	w := gr.author.NewWriter(ctx, bucket, path, overwrite)
	_, err := w.Write(content)
	var reportErr error
	if isErrUnexpected(err) {
		reportErr = err
		gr.logger.WithError(err).WithFields(logrus.Fields{"bucket": bucket, "path": path}).Warn("Uploading info to GCS failed (write)")
	}
	err = w.Close()
	if isErrUnexpected(err) {
		reportErr = err
		gr.logger.WithError(err).WithFields(logrus.Fields{"bucket": bucket, "path": path}).Warn("Uploading info to GCS failed (close)")
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

func (gr *gcsReporter) getJobDestination(pj *prowv1.ProwJob) (bucket, dir string, err error) {
	// The decoration config is always provided for decorated jobs, but many
	// jobs are not decorated, so we guess that we should use the default location
	// for those jobs. This assumption is usually (but not always) correct.
	// The TestGrid configurator uses the same assumption.
	repo := "*"
	if pj.Spec.Refs != nil {
		repo = pj.Spec.Refs.Org + "/" + pj.Spec.Refs.Repo
	}

	ddc := gr.cfg().Plank.GetDefaultDecorationConfigs(repo)

	var gcsConfig *prowv1.GCSConfiguration
	if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.GCSConfiguration != nil {
		gcsConfig = pj.Spec.DecorationConfig.GCSConfiguration
	} else if ddc != nil && ddc.GCSConfiguration != nil {
		gcsConfig = ddc.GCSConfiguration
	} else {
		return "", "", fmt.Errorf("couldn't figure out a GCS config for %q", pj.Spec.Job)
	}

	ps := downwardapi.NewJobSpec(pj.Spec, pj.Status.BuildID, pj.Name)
	_, d, _ := gcsupload.PathsForJob(gcsConfig, &ps, "")

	return gcsConfig.Bucket, d, nil
}

func (gr *gcsReporter) GetName() string {
	return reporterName
}

func (gr *gcsReporter) ShouldReport(pj *prowv1.ProwJob) bool {
	return true
}

func New(cfg config.Getter, storage *storage.Client, dryRun bool) *gcsReporter {
	return newWithAuthor(cfg, storageAuthor{client: storage}, dryRun)
}

func newWithAuthor(cfg config.Getter, author author, dryRun bool) *gcsReporter {
	return &gcsReporter{
		cfg:    cfg,
		dryRun: dryRun,
		logger: logrus.WithField("component", reporterName),
		author: author,
	}
}
