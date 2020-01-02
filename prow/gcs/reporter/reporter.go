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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const reporterName = "gcsreporter"

type gcsReporter struct {
	cfg           config.Getter
	dryRun        bool
	logger        *logrus.Entry
	podClientSets map[string]corev1.CoreV1Interface
	storage       *storage.Client
}

func (gr *gcsReporter) Report(pj *prowv1.ProwJob) ([]*prowv1.ProwJob, error) {
	ctx := context.Background() // TODO: get this from somewhere better?

	_, _, err := gr.getJobDestination(pj)
	if err != nil {
		return []*prowv1.ProwJob{pj}, nil
	}
	// TODO: do something with these errors?
	// TODO: parallelise?
	_ = gr.reportPodInfo(pj, ctx)
	_ = gr.reportFinishedJob(pj, ctx)
	_ = gr.reportProwjob(pj, ctx)
	return []*prowv1.ProwJob{pj}, nil
}

// reportPodInfo reports information about the pod that the job ran in. In particular,
// it reports the pod status, as well as any events concerning the pod.
func (gr *gcsReporter) reportPodInfo(pj *prowv1.ProwJob, ctx context.Context) error {
	// We only report this after a prowjob is complete (and, therefore, pod state is immutable)
	if !pj.Complete() {
		return nil
	}

	pod, err := gr.podClientSets[pj.ClusterName].Pods(gr.cfg().PodNamespace).Get(pj.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	var events *v1.EventList
	if pod != nil {
		// we don't particularly care if this fails.
		events, _ = gr.podClientSets[pj.ClusterName].Events(gr.cfg().PodNamespace).Search(scheme.Scheme, pod)
	}

	outputMap := map[string]interface{}{}
	if pod != nil {
		outputMap["pod"] = pod
	}
	if events != nil && len(events.Items) > 0 {
		outputMap["events"] = events.Items
	}

	output, err := json.Marshal(outputMap)
	if err != nil {
		// This should never happen.
		gr.logger.WithError(err).Warn("Couldn't marshal pod info")
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
	w := b.Object(path.Join(dir, "pod_info.json")).NewWriter(ctx)
	_, err = w.Write(output)
	if err != nil {
		gr.logger.WithError(err).Warn("Uploading pod info to GCS failed (write)")
	}
	err = w.Close()
	if err != nil {
		gr.logger.WithError(err).Warn("Uploading pod info to GCS failed (close)")
	}
	return err
}

// reportFinishedJob uploads a finished.json for the job, iff one did not already exist.
func (gr *gcsReporter) reportFinishedJob(pj *prowv1.ProwJob, ctx context.Context) error {
	// We don't upload a finished.json until we finish.
	if !pj.Complete() {
		return nil
	}

	// We generate and upload this file unconditionally, using a GCS precondition
	// to ensure we don't overwrite files that already exist. This saves us from a)
	// needing to look up the existence of the files and b) any possible races.
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
		gr.logger.Infof("Would upload pod info to %q/%q", bucketName, dir)
		return nil
	}
	b := gr.storage.Bucket(bucketName)
	w := b.Object(path.Join(dir, "finished.json")).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	return gr.writeContent(w, output)
}

func (gr *gcsReporter) reportProwjob(pj *prowv1.ProwJob, ctx context.Context) error {
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

func New(cfg config.Getter, podClientSets map[string]corev1.CoreV1Interface, storage *storage.Client, dryRun bool) *gcsReporter {
	return &gcsReporter{
		cfg:           cfg,
		dryRun:        dryRun,
		logger:        logrus.WithField("component", reporterName),
		podClientSets: podClientSets,
		storage:       storage,
	}
}
