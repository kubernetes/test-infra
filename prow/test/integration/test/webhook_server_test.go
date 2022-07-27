/*
Copyright 2021 The Kubernetes Authors.

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

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"testing"
	"time"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// func cleanUpCluster(prowjob string) error {
// 	err := exec.Command("kubectl", "--context=kind-kind-prow-integration", "delete", "prowjob", prowjob).Run()
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

const (
	validProwJobID = "test-job2"
	invalidProwJobID = "test-job"
)

func TestWebhookServerValidateCluster(t *testing.T) {
	invalidProwJobPath := "./testdata/invalid_prowjob.yaml"
	err := createProwJob(invalidProwJobPath, invalidProwJobID, true)
	if err != nil {
		t.Errorf("could not properly validate prowjob %s %v", invalidProwJobPath, err)
	}
	// err = cleanUpCluster(validProwJobID)
	// if err != nil {
	// 	t.Errorf("could not delete prowjob %s %v", validProwJobID, err)
	// }
}

func createProwJob(path string, prowjobID string, shouldFail bool) error {
	prowJobPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("could not find absolute file path")
	}
	err = exec.Command("kubectl", "--context=kind-kind-prow-integration", "apply", "-f", prowJobPath).Run()
	if shouldFail {
		if err == nil {
			return fmt.Errorf("prowjob has invalid cluster and should not be able to be applied")
		}
	} else {
		if err != nil {
			return fmt.Errorf("error validating prowjob %v", err)
		}
	}
	ctx := context.Background()
	clusterContext := getClusterContext()
	kubeClient, err := NewClients("", clusterContext)
	if err := wait.Poll(time.Second, 20*time.Second, func() (bool, error) {
		pjs := &prowjobv1.ProwJobList{}
		err = kubeClient.List(ctx, pjs, &ctrlruntimeclient.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{"prow.k8s.io/id": prowjobID}),
			Namespace:     defaultNamespace,
		})
		if err != nil {
			return false, fmt.Errorf("failed listing prow jobs: %v", err)
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("error %v", err)
	}
	return nil
}