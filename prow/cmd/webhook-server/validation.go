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
	"io/ioutil"
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

func (wa *webhookAgent) serveValidate(w http.ResponseWriter, r *http.Request) {
    body, err := ioutil.ReadAll(r.Body) 
    if err != nil {
        logrus.WithError(err).Fatal("Unable to read request")
    }
    var prowJob v1.ProwJob
    admissionReview := &v1beta1.AdmissionReview{}
    err = json.Unmarshal(body, admissionReview)
	if err != nil {
        logrus.WithError(err).Fatal("Unable to unmarshal admission review request")
    }
    admissionRequest := admissionReview.Request
    err = json.Unmarshal(admissionRequest.Object.Raw, &prowJob)
    if err != nil {
        logrus.WithError(err).Fatal("Unable to unmarshal admission review request")
    }
    var admissionResponse *v1beta1.AdmissionResponse
    if admissionRequest.Operation == "CREATE" {
        if err := validateProwJobClusterOnCreate(prowJob, wa.statuses); err != nil {
            admissionResponse = createAdmissionResponse(admissionRequest.UID, false, "Denied", err.Error())
        } else {
            admissionResponse = createAdmissionResponse(admissionRequest.UID, true, "Allowed", "")
        }
    }
    admissionReview.Response = admissionResponse
    resp, err := json.Marshal(admissionReview)
    if err != nil {
        logrus.WithError(err).Fatal("Unable to marshal response")
    }
    if _, err := w.Write(resp); err != nil {
		logrus.WithError(err).Fatal("Unable to write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func validateProwJobClusterOnCreate(prowJob v1.ProwJob, statuses map[string]plank.ClusterStatus) (error) {
    if  prowJob.Spec.Cluster!= "" && prowJob.Spec.Cluster != kube.DefaultClusterAlias && agentsNotSupportingCluster.Has(string(prowJob.Spec.Agent)) {
		return fmt.Errorf("%s: cannot set cluster field if agent is %s", prowJob.Name, prowJob.Spec.Agent)
	}
	if statuses != nil {
		status, ok := statuses[prowJob.Spec.Cluster]
		if !ok {
			return fmt.Errorf("job configuration for %q specifies unknown 'cluster' value %q", prowJob.Name, prowJob.Spec.Cluster)
		}
		if status != plank.ClusterStatusReachable {
			logrus.Warnf("Job configuration for %q specifies cluster %q which cannot be reached from Plank. Status: %q", prowJob.Name, prowJob.Spec.Cluster, status)
		}
    }
    return nil
}

func createAdmissionResponse(uid types.UID, allowed bool, message string, reason string) *v1beta1.AdmissionResponse {
    var ar *v1beta1.AdmissionResponse
    if allowed {
        ar = &v1beta1.AdmissionResponse {
            UID: uid,
            Allowed: allowed,
            Result: &apiv1.Status{
                Message: message,
            },
        }
    } else {
        ar = &v1beta1.AdmissionResponse {
            UID: uid,
            Allowed: allowed,
            Result: &apiv1.Status{
                Message: message,
                Reason: apiv1.StatusReason(reason),
            },
        }
    }
    return ar
}

