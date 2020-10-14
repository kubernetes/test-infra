/*
Copyright 2017 The Kubernetes Authors.

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

package pjutil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	pjapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowconfig "k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

// getJobArtifactsURL returns the artifacts URL for the given job
func getJobArtifactsURL(prowJob *pjapi.ProwJob, config *prowconfig.Config) string {
	var identifier string
	if prowJob.Spec.Refs != nil {
		identifier = fmt.Sprintf("%s/%s", prowJob.Spec.Refs.Org, prowJob.Spec.Refs.Repo)
	} else {
		identifier = fmt.Sprintf("%s/%s", prowJob.Spec.ExtraRefs[0].Org, prowJob.Spec.ExtraRefs[0].Repo)
	}
	spec := downwardapi.NewJobSpec(prowJob.Spec, prowJob.Status.BuildID, prowJob.Name)
	jobBasePath, _, _ := gcsupload.PathsForJob(config.Plank.GetDefaultDecorationConfigs(identifier).GCSConfiguration, &spec, "")
	return fmt.Sprintf("%s%s/%s",
		config.Deck.Spyglass.GCSBrowserPrefix,
		config.Plank.GetDefaultDecorationConfigs(identifier).GCSConfiguration.Bucket,
		jobBasePath,
	)
}

// Calls toJSON method on a jobResult type and writes it to the output path
func writeResultOutput(prowjobResult JobResult, outputPath string, fileSystem afero.Fs) error {
	j, err := prowjobResult.toJSON()
	if err != nil {
		return fmt.Errorf("unable to marshal prowjob result to JSON: %w", err)
	}

	afs := afero.Afero{Fs: fileSystem}
	if outputPath != "" {
		err = afs.WriteFile(outputPath, j, 0755)
		if err != nil {
			logrus.WithField("output path", outputPath).Error("error writing to output file")
			return err
		}
	} else {
		fmt.Println(string(j))
	}

	return nil
}

type JobResult interface {
	toJSON() ([]byte, error)
}

type prowjobResult struct {
	Status       pjapi.ProwJobState `json:"status"`
	ArtifactsURL string             `json:"prowjob_artifacts_url"`
	URL          string             `json:"prowjob_url"`
}

func (p *prowjobResult) toJSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "    ")
}

func TriggerProwJob(o prowflagutil.KubernetesOptions, prowjob *pjapi.ProwJob, config *prowconfig.Config, envVars map[string]string, fileSystem afero.Fs, dryRun bool, outputPath string) error {
	logrus.Info("getting cluster config")
	pjclient, err := o.ProwJobClient(config.ProwJobNamespace, dryRun)
	if err != nil {
		return fmt.Errorf("failed getting prowjob client: %w", err)
	}

	logrus.WithFields(ProwJobFields(prowjob)).Info("submitting a new prowjob")
	created, err := pjclient.Create(context.TODO(), prowjob, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to submit the prowjob: %w", err)
	}

	logger := logrus.WithFields(ProwJobFields(created))
	logger.Info("submitted the prowjob, waiting for its result")

	selector := fields.SelectorFromSet(map[string]string{"metadata.name": created.Name})

	for {
		w, err := pjclient.Watch(context.TODO(), metav1.ListOptions{FieldSelector: selector.String()})
		if err != nil {
			return fmt.Errorf("failed to create watch for ProwJobs: %w", err)
		}

		for event := range w.ResultChan() {
			prowJob, ok := event.Object.(*pjapi.ProwJob)
			if !ok {
				return fmt.Errorf("received an unexpected object from Watch: object-type %s", fmt.Sprintf("%T", event.Object))
			}

			prowJobArtifactsURL := getJobArtifactsURL(prowJob, config)

			switch prowJob.Status.State {
			case pjapi.FailureState, pjapi.AbortedState, pjapi.ErrorState:
				pjr := &prowjobResult{
					Status:       prowJob.Status.State,
					ArtifactsURL: prowJobArtifactsURL,
					URL:          prowJob.Status.URL,
				}
				err = writeResultOutput(pjr, outputPath, fileSystem)
				if err != nil {
					logrus.Error("Unable to write prowjob result to file")
				}
				logrus.Warn("job failed")
				return nil
			case pjapi.SuccessState:
				pjr := &prowjobResult{
					Status:       prowJob.Status.State,
					ArtifactsURL: prowJobArtifactsURL,
					URL:          prowJob.Status.URL,
				}
				err = writeResultOutput(pjr, outputPath, fileSystem)
				if err != nil {
					logrus.Error("Unable to write prowjob result to file")
				}
				logrus.Info("job succeeded")
				return nil
			}
		}
	}
}
