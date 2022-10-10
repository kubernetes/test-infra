/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sirupsen/logrus"
	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/api/admission/v1beta1"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func (wa *webhookAgent) serveMutate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logrus.WithError(err).Info("unable to read request")
		http.Error(w, fmt.Sprintf("bad request %v", err), http.StatusBadRequest)
		return
	}
	admissionReview := &v1beta1.AdmissionReview{}
	err = json.Unmarshal(body, admissionReview)
	if err != nil {
		logrus.WithError(err).Info("unable to unmarshal admission review request")
		http.Error(w, fmt.Sprintf("unable to unmarshal admission review request %v", err), http.StatusBadRequest)
		return
	}
	admissionRequest := admissionReview.Request
	var prowJob v1.ProwJob
	err = json.Unmarshal(admissionRequest.Object.Raw, &prowJob)
	if err != nil {
		logrus.WithError(err).Info("unable to prowjob from request")
		http.Error(w, fmt.Sprintf("unable to unmarshal prowjob %v", err), http.StatusBadRequest)
		return
	}
	var mutatedProwJobPatch []byte
	if admissionRequest.Operation == "CREATE" {
		mutatedProwJobPatch, err = generateMutatingPatch(&prowJob, wa.plank)
		if err != nil {
			logrus.WithError(err).Info("unable to return mutated prowjob patch")
			http.Error(w, fmt.Sprintf("unable to return mutated prowjob patch %v", err), http.StatusInternalServerError)
			return
		}
	}
	admissionReview.Response = createMutatingAdmissionResponse(admissionRequest.UID, mutatedProwJobPatch)
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		logrus.WithError(err).Info("unable to marshal response")
		http.Error(w, fmt.Sprintf("unable to marshal mutated prowjob patch %v", err), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(resp); err != nil {
		logrus.WithError(err).Info("unable to write response")
		http.Error(w, fmt.Sprintf("unable to write response: %v", err), http.StatusInternalServerError)
		return
	}
}

func createMutatingAdmissionResponse(uid types.UID, patch []byte) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		UID:     uid,
		Allowed: true,
		Result: &apiv1.Status{
			Message: accepted,
		},
		Patch: patch,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func generateMutatingPatch(prowJob *v1.ProwJob, plank config.Plank) ([]byte, error) {
	var defDecorationConfig *v1.DecorationConfig
	var patchBytes []byte
	if prowJob.Spec.Type == v1.PeriodicJob {
		var repo string
		if len(prowJob.Spec.ExtraRefs) > 0 {
			repo = fmt.Sprintf("%s/%s", prowJob.Spec.ExtraRefs[0].Org, prowJob.Spec.ExtraRefs[0].Repo)
		}
		defDecorationConfig = plank.GuessDefaultDecorationConfigWithJobDC(repo, prowJob.Spec.Cluster, prowJob.Spec.DecorationConfig)
	} else {
		defDecorationConfig = plank.GuessDefaultDecorationConfig(prowJob.Spec.Refs.Repo, prowJob.Spec.Cluster)
	}
	prowJobCopy := prowJob.DeepCopy()
	prowJobCopy.Spec.DecorationConfig = prowJobCopy.Spec.DecorationConfig.ApplyDefault(defDecorationConfig)
	originalProwJobJSON, err := json.Marshal(prowJob)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal prowjob %v", err)
	}
	mutatedProwJobJSON, err := json.Marshal(prowJobCopy)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal prowjob %v", err)
	}
	patch, err := jsonpatch.CreatePatch(originalProwJobJSON, mutatedProwJobJSON)
	if err != nil {
		return nil, fmt.Errorf("unable to create json patch")
	}
	patchBytes, err = json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal patch")
	}

	return patchBytes, nil
}
