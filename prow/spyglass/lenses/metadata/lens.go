/*
Copyright 2018 The Kubernetes Authors.

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

// Package metadata provides a metadata viewer for Spyglass
package metadata

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"fmt"
	"html/template"
	"path/filepath"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	k8sreporter "k8s.io/test-infra/prow/crier/reporters/gcs/kubernetes"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name     = "metadata"
	title    = "Metadata"
	priority = 0
)

// Lens is the implementation of a metadata-rendering Spyglass lens.
type Lens struct{}

func init() {
	lenses.RegisterLens(Lens{})
}

// Config returns the lens's configuration.
func (lens Lens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Title:     title,
		Name:      name,
		Priority:  priority,
		HideTitle: true,
	}
}

// Header renders the <head> from template.html.
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	t, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("<!-- FAILED LOADING HEADER: %v -->", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "header", nil); err != nil {
		return fmt.Sprintf("<!-- FAILED EXECUTING HEADER TEMPLATE: %v -->", err)
	}
	return buf.String()
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

// Body creates a view for prow job metadata.
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	var buf bytes.Buffer
	type MetadataViewData struct {
		StartTime    time.Time
		FinishedTime time.Time
		Finished     bool
		Passed       bool
		Errored      bool
		Elapsed      time.Duration
		Hint         string
		Metadata     map[string]interface{}
	}
	metadataViewData := MetadataViewData{}
	started := gcs.Started{}
	finished := gcs.Finished{}
	for _, a := range artifacts {
		read, err := a.ReadAll()
		if err != nil {
			logrus.WithError(err).Error("Failed reading from artifact.")
		}
		switch a.JobPath() {
		case prowv1.StartedStatusFile:
			if err = json.Unmarshal(read, &started); err != nil {
				logrus.WithError(err).Error("Error unmarshaling started.json")
			}
			metadataViewData.StartTime = time.Unix(started.Timestamp, 0)
		case prowv1.FinishedStatusFile:
			if err = json.Unmarshal(read, &finished); err != nil {
				logrus.WithError(err).Error("Error unmarshaling finished.json")
			}
			metadataViewData.Finished = true
			if finished.Timestamp != nil {
				metadataViewData.FinishedTime = time.Unix(*finished.Timestamp, 0)
			}
			if finished.Passed != nil {
				metadataViewData.Passed = *finished.Passed
			} else {
				metadataViewData.Passed = finished.Result == "SUCCESS"
			}
		case "podinfo.json":
			metadataViewData.Hint = hintFromPodInfo(read)
		case "prowjob.json":
			// Only show the prowjob-based hint if we don't have a pod-based one
			// (the pod-based ones are probably more useful when they exist)
			if metadataViewData.Hint == "" {
				hint, errored := hintFromProwJob(read)
				metadataViewData.Hint = hint
				metadataViewData.Errored = errored
			}
		}
	}

	if !metadataViewData.StartTime.IsZero() {
		if metadataViewData.FinishedTime.IsZero() {
			metadataViewData.Elapsed = time.Since(metadataViewData.StartTime)
		} else {
			metadataViewData.Elapsed =
				metadataViewData.FinishedTime.Sub(metadataViewData.StartTime)
		}
		metadataViewData.Elapsed = metadataViewData.Elapsed.Round(time.Second)
	}

	metadataViewData.Metadata = map[string]interface{}{"node": started.Node}

	metadatas := []metadata.Metadata{started.Metadata, finished.Metadata}
	for _, m := range metadatas {
		for k, v := range lens.flattenMetadata(m) {
			metadataViewData.Metadata[k] = v
		}
	}

	metadataTemplate, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("Failed to load template: %v", err)
	}

	if err := metadataTemplate.ExecuteTemplate(&buf, "body", metadataViewData); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}

var failedMountRegex = regexp.MustCompile(`MountVolume.SetUp failed for volume "(.+?)" : (.+)`)

func hintFromPodInfo(buf []byte) string {
	var report k8sreporter.PodReport
	if err := json.Unmarshal(buf, &report); err != nil {
		logrus.WithError(err).Info("Failed to decode podinfo.json")
		// This error isn't worth highlighting here, and will be reported in the
		// podinfo lens if that is enabled.
		return ""
	}

	// We're more likely to pick a relevant event if we use the last ones first.
	sort.Slice(report.Events, func(i, j int) bool {
		a := &report.Events[i]
		b := &report.Events[j]
		return b.LastTimestamp.Before(&a.LastTimestamp)
	})

	// If the pod completed successfully there's probably not much to say.
	if report.Pod.Status.Phase == v1.PodSucceeded {
		return ""
	}
	// Check if we have any images that didn't pull
	for _, s := range append(report.Pod.Status.InitContainerStatuses, report.Pod.Status.ContainerStatuses...) {
		if s.State.Waiting != nil && (s.State.Waiting.Reason == "ImagePullBackOff" || s.State.Waiting.Reason == "ErrImagePull") {
			return fmt.Sprintf("The %s container could not start because it could not pull %q. Check your images. Full message: %q", s.Name, s.Image, s.State.Waiting.Message)
		}
	}
	// Check if we're trying to mount a volume
	if report.Pod.Status.Phase == v1.PodPending {
		failedMount := false
		for _, e := range report.Events {
			if e.Reason == "FailedMount" {
				failedMount = true
				if strings.HasPrefix(e.Message, "MountVolume.SetUp") {
					// Annoyingly, parsing this message is the only way to get this information.
					// If we can't parse it, we'll fall through to a generic bad volume message below.
					results := failedMountRegex.FindStringSubmatch(e.Message)
					if results == nil {
						continue
					}
					return fmt.Sprintf("The pod could not start because it could not mount the volume %q: %s", results[1], results[2])
				}
			}
		}
		if failedMount {
			return "The job could not started because one or more of the volumes could not be mounted."
		}
	}
	// Check if we cannot be scheduled
	// This is unlikely - we only outright fail if a pod is actually scheduled to a node that can't support it.
	if report.Pod.Status.Phase == v1.PodFailed && report.Pod.Status.Reason == "MatchNodeSelector" {
		return "The job could not start because it was scheduled to a node that does not satisfy its NodeSelector"
	}
	// Usually we would fail to schedule it at all, so it will be pending forever.
	if report.Pod.Status.Phase == v1.PodPending {
		for _, e := range report.Events {
			if e.Reason == "FailedScheduling" {
				return fmt.Sprintf("There are no nodes that your pod can schedule to - check your requests, tolerations, and node selectors (%s)", e.Message)
			}
		}
	}

	// There are a bunch of fun ways for the node to fail that we've seen before
	for _, e := range report.Events {
		if e.Reason == "FailedCreatePodSandbox" || e.Reason == "FailedSync" {
			return "The job may have executed on an unhealthy node. Contact your prow maintainers with a link to this page or check the detailed pod information."
		}
	}

	// There are cases where initContainers failed to start
	var msgs []string
	for _, ic := range report.Pod.Status.InitContainerStatuses {
		if ic.Ready {
			continue
		}
		var msg string
		// Init container not ready by the time this job failed
		// The 3 different states should be mutually exclusive, if it happens
		// that there are more than one, use the most severe one
		if state := ic.State.Terminated; state != nil {
			msg = fmt.Sprintf("state: terminated, reason: %q, message: %q", state.Reason, state.Message)
		} else if state := ic.State.Waiting; state != nil {
			msg = fmt.Sprintf("state: waiting, reason: %q, message: %q", state.Reason, state.Message)
		} else if state := ic.State.Running; state != nil { // This shouldn't happen at all, just in case.
			logrus.WithField("pod", report.Pod.Name).WithField("container", ic.Name).Warning("Init container is running but not ready")
		}
		msgs = append(msgs, fmt.Sprintf("Init container %s not ready: (%s)", ic.Name, msg))
	}
	return strings.Join(msgs, "\n")
}

func hintFromProwJob(buf []byte) (string, bool) {
	var pj prowv1.ProwJob
	if err := json.Unmarshal(buf, &pj); err != nil {
		logrus.WithError(err).Info("Failed to decode prowjob.json")
		return "", false
	}

	if pj.Status.State == prowv1.ErrorState {
		return fmt.Sprintf("Job execution failed: %s", pj.Status.Description), true
	}

	return "", false
}

// flattenMetadata flattens the metadata for use by Body.
func (lens Lens) flattenMetadata(metadata map[string]interface{}) map[string]string {
	results := map[string]string{}

	for k1, v1 := range metadata {
		if s, ok := v1.(map[string]interface{}); ok && len(s) > 0 {
			subObjectResults := lens.flattenMetadata(s)
			for k2, v2 := range subObjectResults {
				results[fmt.Sprintf("%s.%s", k1, k2)] = v2
			}
		} else if s, ok := v1.(string); ok && v1 != "" { // We ought to consider relaxing this so that non-strings will be considered
			results[k1] = s
		}
	}

	return results
}
