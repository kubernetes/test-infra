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

	"k8s.io/api/admission/v1beta1"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plank"
)

var agentsNotSupportingCluster = sets.NewString("jenkins")

const (
	denied   = "DENIED"
	accepted = "ACCEPTED"
)

func (wa *webhookAgent) serveValidate(w http.ResponseWriter, r *http.Request) {
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
	var admissionResponse *v1beta1.AdmissionResponse
	if admissionRequest.Operation == "CREATE" {
		if err := validateProwJobClusterOnCreate(prowJob, wa.statuses); err != nil {
			admissionResponse = createValidatingAdmissionResponse(admissionRequest.UID, err)
		} else {
			admissionResponse = createValidatingAdmissionResponse(admissionRequest.UID, nil)
		}
	}
	admissionReview.Response = admissionResponse
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		logrus.WithError(err).Info("unable to marshal response")
		http.Error(w, fmt.Sprintf("unable to unmarshal prowjob %v", err), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(resp); err != nil {
		logrus.WithError(err).Info("unable to write response")
		http.Error(w, fmt.Sprintf("unable to write response: %v", err), http.StatusInternalServerError)
		return
	}
}

func validateProwJobClusterOnCreate(prowJob v1.ProwJob, statuses map[string]plank.ClusterStatus) error {
	if prowJob.Spec.Cluster != "" && prowJob.Spec.Cluster != kube.DefaultClusterAlias && agentsNotSupportingCluster.Has(string(prowJob.Spec.Agent)) {
		return fmt.Errorf("%s: cannot set cluster field if agent is %s", prowJob.Name, prowJob.Spec.Agent)
	}
	if prowJob.Spec.Agent == v1.KubernetesAgent {
		_, ok := statuses[prowJob.ClusterAlias()]
		if !ok {
			return fmt.Errorf("job configuration for %q specifies unknown 'cluster' value %q", prowJob.Name, prowJob.ClusterAlias())
		}
	}
	return nil
}

func createValidatingAdmissionResponse(uid types.UID, err error) *v1beta1.AdmissionResponse {
	var ar *v1beta1.AdmissionResponse
	var result *apiv1.Status
	if err != nil {
		result = &apiv1.Status{
			Message: denied,
			Reason:  apiv1.StatusReason(err.Error()),
		}
	} else {
		result = &apiv1.Status{
			Message: accepted,
		}
	}
	ar = &v1beta1.AdmissionResponse{
		UID:     uid,
		Allowed: err == nil,
		Result:  result,
	}
	return ar
}
