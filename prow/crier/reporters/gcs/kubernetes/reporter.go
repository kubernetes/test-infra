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

package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"path"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/util"
	"k8s.io/test-infra/prow/io"
)

const reporterName = "gcsk8sreporter"

type gcsK8sReporter struct {
	cfg            config.Getter
	dryRun         bool
	logger         *logrus.Entry
	author         util.Author
	rg             resourceGetter
	reportFraction float32
}

type PodReport struct {
	Pod    *v1.Pod    `json:"pod,omitempty"`
	Events []v1.Event `json:"events,omitempty"`
}

type resourceGetter interface {
	GetPod(cluster, namespace, name string) (*v1.Pod, error)
	GetEvents(cluster, namespace string, pod *v1.Pod) ([]v1.Event, error)
}

type k8sResourceGetter struct {
	podClientSets map[string]corev1.CoreV1Interface
}

func (rg k8sResourceGetter) GetPod(cluster, namespace, name string) (*v1.Pod, error) {
	if _, ok := rg.podClientSets[cluster]; !ok {
		return nil, fmt.Errorf("couldn't find cluster %q", cluster)
	}
	return rg.podClientSets[cluster].Pods(namespace).Get(name, metav1.GetOptions{})
}

func (rg k8sResourceGetter) GetEvents(cluster, namespace string, pod *v1.Pod) ([]v1.Event, error) {
	if _, ok := rg.podClientSets[cluster]; !ok {
		return nil, fmt.Errorf("couldn't find cluster %q", cluster)
	}
	events, err := rg.podClientSets[cluster].Events(namespace).Search(scheme.Scheme, pod)
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

func (gr *gcsK8sReporter) Report(pj *prowv1.ProwJob) ([]*prowv1.ProwJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // TODO: pass through a global context?
	defer cancel()

	_, _, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		gr.logger.Warnf("Not uploading %q (%s#%s) because we couldn't find a destination: %v", pj.Name, pj.Spec.Job, pj.Status.BuildID, err)
		return []*prowv1.ProwJob{pj}, nil
	}

	return []*prowv1.ProwJob{pj}, gr.reportPodInfo(ctx, pj)
}

func (gr *gcsK8sReporter) reportPodInfo(ctx context.Context, pj *prowv1.ProwJob) error {
	// We only report this after a prowjob is complete (and, therefore, pod state is immutable)
	if !pj.Complete() {
		return errors.New("cannot report incomplete jobs")
	}

	pod, err := gr.rg.GetPod(pj.Spec.Cluster, gr.cfg().PodNamespace, pj.Name)
	if err != nil {
		// If we return an error we will be retried ~indefinitely. Given that permanent errors
		// are expected (pods will be garbage collected), this isn't useful. Instead, just
		// go along with it.
		gr.logger.WithError(err).Infof("Couldn't fetch info for pod %s", pj.Name)
		pod = nil
	}

	var events []v1.Event
	if pod != nil {
		events, err = gr.rg.GetEvents(pj.Spec.Cluster, gr.cfg().PodNamespace, pod)
		if err != nil {
			gr.logger.WithError(err).Infof("Couldn't fetch events for pod %s", pj.Name)
		}
	}

	if pod == nil && len(events) == 0 {
		gr.logger.Infof("Not reporting on job %q because we could fetch neither pod nor events", pj.Name)
		return nil
	}

	report := PodReport{
		Pod:    pod,
		Events: events,
	}

	output, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		// This should never happen.
		gr.logger.WithError(err).Warn("Couldn't marshal pod info")
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("couldn't get job destination: %v", err)
	}

	if gr.dryRun {
		gr.logger.Infof("Would upload pod info to %q/%q", bucketName, dir)
		return nil
	}

	return util.WriteContent(ctx, gr.logger, gr.author, bucketName, path.Join(dir, "podinfo.json"), true, output)
}

func (gr *gcsK8sReporter) GetName() string {
	return reporterName
}

func (gr *gcsK8sReporter) ShouldReport(pj *prowv1.ProwJob) bool {
	// This reporting only makes sense for the Kubernetes agent (otherwise we don't
	// have a pod to look up). It is only particularly useful for us to look at
	// complete jobs that have a build ID.
	if pj.Spec.Agent != prowv1.KubernetesAgent || !pj.Complete() || pj.Status.BuildID == "" {
		return false
	}

	// For ramp-up purposes, we can report only on a subset of jobs.
	if gr.reportFraction < 1.0 {
		// Assume the names are opaque and take the CRC-32C checksum of it.
		// (Why CRC-32C? It's sufficiently well distributed and fast)
		crc := crc32.Checksum([]byte(pj.Name), crc32.MakeTable(crc32.Castagnoli))
		if crc > uint32(math.MaxUint32*gr.reportFraction) {
			return false
		}
	}

	return true
}

func New(cfg config.Getter, opener io.Opener, podClientSets map[string]corev1.CoreV1Interface, reportFraction float32, dryRun bool) *gcsK8sReporter {
	return internalNew(cfg, util.StorageAuthor{Opener: opener}, k8sResourceGetter{podClientSets: podClientSets}, reportFraction, dryRun)
}

func internalNew(cfg config.Getter, author util.Author, rg resourceGetter, reportFraction float32, dryRun bool) *gcsK8sReporter {
	return &gcsK8sReporter{
		cfg:            cfg,
		dryRun:         dryRun,
		logger:         logrus.WithField("component", reporterName),
		author:         author,
		rg:             rg,
		reportFraction: reportFraction,
	}
}
