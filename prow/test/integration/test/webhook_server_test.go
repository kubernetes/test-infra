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
	"os/exec"
	"path/filepath"

	"testing"
)

func cleanUpCluster(prowjob string) error {
	err := exec.Command("kubectl", "--context=kind-kind-prow-integration", "delete", "prowjob", prowjob).Run()
	if err != nil {
		return err
	}
	return nil
}

const (
	validProwJobID   = "test-job2"
)

func TestWebhookServerValidateCluster(t *testing.T) {
	invalidProwJobPath := "./testdata/invalid_prowjob.yaml"
	validProwJobPath := "./testdata/valid_prowjob.yaml"

	absInvalidProwJobPath, err := filepath.Abs(invalidProwJobPath)
	if err != nil {
		t.Errorf("could not find absolute file path")
	}
	absValidProwJobPath, err := filepath.Abs(validProwJobPath)
	if err != nil {
		t.Errorf("could not find absolute file path")
	}
	//prowjob should fail and produce error
	err = exec.Command("kubectl", "--context=kind-kind-prow-integration", "apply", "-f", absInvalidProwJobPath).Run()
	if err == nil {
		t.Errorf("prowjob was not properly validated")
	}
	//prowjob should be created successfully
	err = exec.Command("kubectl", "--context=kind-kind-prow-integration", "apply", "-f", absValidProwJobPath).Run()
	if err != nil {
		t.Errorf("prowjob was not properly validated")
	}
	//cleanup prowjob that was created
	err = cleanUpCluster(validProwJobID)
	if err != nil {
		t.Errorf("could not delete prowjob %s", validProwJobID)
	}
}
