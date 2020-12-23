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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/util"
	kubernetesreporterapi "k8s.io/test-infra/prow/crier/reporters/gcs/kubernetes/api"
	"k8s.io/test-infra/prow/io"
)

type gcsK8sReporter struct {
	cfg            config.Getter
	dryRun         bool
	author         util.Author
	rg             resourceGetter
	reportFraction float32
}

type PodReport struct {
	Pod    *v1.Pod    `json:"pod,omitempty"`
	Events []v1.Event `json:"events,omitempty"`
}

type resourceGetter interface {
	GetPod(ctx context.Context, cluster, namespace, name string) (*v1.Pod, error)
	GetEvents(cluster, namespace string, pod *v1.Pod) ([]v1.Event, error)
	PatchPod(ctx context.Context, cluster, namespace, name string, pt types.PatchType, data []byte) error
}

type k8sResourceGetter struct {
	podClientSets map[string]corev1.CoreV1Interface
}

func (rg k8sResourceGetter) GetPod(ctx context.Context, cluster, namespace, name string) (*v1.Pod, error) {
	if _, ok := rg.podClientSets[cluster]; !ok {
		return nil, fmt.Errorf("couldn't find cluster %q", cluster)
	}
	return rg.podClientSets[cluster].Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (rg k8sResourceGetter) PatchPod(ctx context.Context, cluster, namespace, name string, pt types.PatchType, data []byte) error {
	if _, ok := rg.podClientSets[cluster]; !ok {
		return fmt.Errorf("couldn't find cluster %q", cluster)
	}

	_, err := rg.podClientSets[cluster].Pods(namespace).Patch(ctx, name, pt, data, metav1.PatchOptions{})
	return err
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

func (gr *gcsK8sReporter) Report(log *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error) {
	result, err := gr.report(log, pj)
	return []*prowv1.ProwJob{pj}, result, err
}

func (gr *gcsK8sReporter) report(log *logrus.Entry, pj *prowv1.ProwJob) (*reconcile.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // TODO: pass through a global context?
	defer cancel()

	// Check if we have a destination before adding a finalizer so we don't add
	// one that we'll never remove.
	_, _, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		log.WithError(err).Warn("Not uploading because we couldn't find a destination")
		return nil, nil
	}

	if !pj.Complete() && pj.Status.State != prowv1.AbortedState {
		if err := gr.addFinalizer(ctx, pj); err != nil {
			return nil, fmt.Errorf("failed to add finalizer to pod: %w", err)
		}
		return nil, nil
	}

	// Aborted jobs are not completed initially
	if !pj.Complete() {
		log.Debug("Requeuing aborted job that is not complete.")
		return &reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return nil, gr.reportPodInfo(ctx, log, pj)
}

func (gr *gcsK8sReporter) addFinalizer(ctx context.Context, pj *prowv1.ProwJob) error {
	pod, err := gr.rg.GetPod(ctx, pj.Spec.Cluster, gr.cfg().PodNamespace, pj.Name)
	if err != nil {
		return fmt.Errorf("failed to get pod %s: %w", pj.Name, err)
	}

	if pod.DeletionTimestamp != nil {
		return nil
	}

	finalizers := sets.NewString(pod.Finalizers...)
	if finalizers.Has(kubernetesreporterapi.FinalizerName) {
		return nil
	}

	originalPod := pod.DeepCopy()
	pod.Finalizers = finalizers.Insert(kubernetesreporterapi.FinalizerName).List()
	patch := ctrlruntimeclient.MergeFrom(originalPod)
	patchData, err := patch.Data(pod)
	if err != nil {
		return fmt.Errorf("failed to construct patch: %w", err)
	}

	if err := gr.rg.PatchPod(ctx, pj.Spec.Cluster, pod.Namespace, pod.Name, patch.Type(), patchData); err != nil {
		return fmt.Errorf("failed to patch pod: %w", err)
	}

	return nil
}

func (gr *gcsK8sReporter) reportPodInfo(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) error {
	// We only report this after a prowjob is complete (and, therefore, pod state is immutable)
	if !pj.Complete() {
		return errors.New("cannot report incomplete jobs")
	}

	pod, err := gr.rg.GetPod(ctx, pj.Spec.Cluster, gr.cfg().PodNamespace, pj.Name)
	if err != nil {
		// If we return an error we will be retried ~indefinitely. Given that permanent errors
		// are expected (pods will be garbage collected), this isn't useful. Instead, just
		// go along with it.
		log.WithError(err).Info("Couldn't fetch pod")
		pod = nil
	}

	var events []v1.Event
	if pod != nil {
		events, err = gr.rg.GetEvents(pj.Spec.Cluster, gr.cfg().PodNamespace, pod)
		if err != nil {
			log.WithError(err).Info("Couldn't fetch events for pod")
		}
	}

	if pod == nil && len(events) == 0 {
		log.Info("Not reporting job because we could fetch neither pod nor events")
		return nil
	}

	report := PodReport{
		Pod:    pod,
		Events: events,
	}

	output, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		// This should never happen.
		log.WithError(err).Warn("Couldn't marshal pod info")
	}

	bucketName, dir, err := util.GetJobDestination(gr.cfg, pj)
	if err != nil {
		return fmt.Errorf("couldn't get job destination: %v", err)
	}

	if gr.dryRun {
		log.WithFields(logrus.Fields{"bucketName": bucketName, "dir": dir}).Info("Would upload pod info")
		return nil
	}

	if err := util.WriteContent(ctx, log, gr.author, bucketName, path.Join(dir, "podinfo.json"), true, output); err != nil {
		return fmt.Errorf("failed to upload pod manifest to object storage: %w", err)
	}

	if pod == nil {
		return nil
	}

	if err := gr.removeFinalizer(ctx, pj.Spec.Cluster, pod); err != nil {
		return fmt.Errorf("failed to remove %s finalizer: %w", kubernetesreporterapi.FinalizerName, err)
	}

	return nil
}

func (gr *gcsK8sReporter) removeFinalizer(ctx context.Context, cluster string, pod *v1.Pod) error {
	finalizers := sets.NewString(pod.Finalizers...)
	if !finalizers.Has(kubernetesreporterapi.FinalizerName) {
		return nil
	}

	oldPod := pod.DeepCopy()
	pod.Finalizers = finalizers.Delete(kubernetesreporterapi.FinalizerName).List()
	patch := ctrlruntimeclient.MergeFrom(oldPod)
	rawPatch, err := patch.Data(pod)
	if err != nil {
		return fmt.Errorf("failed to construct patch: %w", err)
	}

	if err := gr.rg.PatchPod(ctx, cluster, pod.Namespace, pod.Name, patch.Type(), rawPatch); err != nil {
		return fmt.Errorf("failed to patch pod: %w", err)
	}

	return nil
}

func (gr *gcsK8sReporter) GetName() string {
	return kubernetesreporterapi.ReporterName
}

func (gr *gcsK8sReporter) ShouldReport(_ *logrus.Entry, pj *prowv1.ProwJob) bool {
	// This reporting only makes sense for the Kubernetes agent (otherwise we don't
	// have a pod to look up). It is only particularly useful for us to look at
	// complete jobs that have a build ID.
	if pj.Spec.Agent != prowv1.KubernetesAgent || pj.Status.PendingTime == nil || pj.Status.BuildID == "" {
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
		author:         author,
		rg:             rg,
		reportFraction: reportFraction,
	}
}
