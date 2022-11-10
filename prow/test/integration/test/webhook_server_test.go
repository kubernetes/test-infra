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
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"sigs.k8s.io/yaml"
)

func cleanUpCluster(prowjob string) error {
	err := exec.Command("kubectl", "--context="+getClusterContext(), "delete", "prowjob", prowjob).Run()
	if err != nil {
		return err
	}
	return nil
}

const (
	validProwJobID       = "test-job2"
	validBareProwJobID   = "test-mutate-job"
	validBareProwJobPath = "./testdata/valid_bare_prowjob.yaml"
	invalidProwJobPath   = "./testdata/invalid_prowjob.yaml"
	validProwJobPath     = "./testdata/valid_prowjob.yaml"
)

func TestWebhookServerValidateCluster(t *testing.T) {
	t.Cleanup(func() {
		err := cleanUpCluster(validProwJobID)
		if err != nil {
			logrus.WithError(err).Infof("could not delete prowjob %s", validProwJobID)
		}
	})

	absInvalidProwJobPath, err := filepath.Abs(invalidProwJobPath)
	if err != nil {
		t.Fatalf("could not find absolute file path")
	}
	absValidProwJobPath, err := filepath.Abs(validProwJobPath)
	if err != nil {
		t.Fatalf("could not find absolute file path")
	}
	//prowjob should fail and produce error
	out, err := exec.Command("kubectl", "--context="+getClusterContext(), "apply", "-f", absInvalidProwJobPath).CombinedOutput()
	if err == nil {
		t.Fatalf("prowjob was not properly validated. Output: %s", string(out))
	}
	//prowjob should be created successfully
	out, err = exec.Command("kubectl", "--context="+getClusterContext(), "apply", "-f", absValidProwJobPath).CombinedOutput()
	if err != nil {
		t.Fatalf("prowjob was not properly validated. Error: %v Output: %s", err, string(out))
	}
}

func TestWebhookServerMutateProwjob(t *testing.T) {
	t.Cleanup(func() {
		err := cleanUpCluster(validBareProwJobID)
		if err != nil {
			logrus.WithError(err).Infof("could not delete prowjob %s", validProwJobID)
		}
	})

	absValidBareProwJobPath, err := filepath.Abs(validBareProwJobPath)
	if err != nil {
		t.Fatalf("could not find absolute file path")
	}
	var mutatedProwJob v1.ProwJob
	//prowjob should be created successfully
	out, err := exec.Command("kubectl", "--context="+getClusterContext(), "apply", "-f", absValidBareProwJobPath).CombinedOutput()
	if err != nil {
		t.Fatalf("prowjob was not properly validated. Error: %v Output: %s", err, string(out))
	}
	key := types.NamespacedName{
		Namespace: defaultNamespace,
		Name:      validBareProwJobID,
	}
	clusterContext := getClusterContext()
	t.Logf("Creating client for cluster: %s", clusterContext)
	kubeClient, err := NewClients("", clusterContext)
	if err != nil {
		t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
	}
	ctx := context.Background()
	if err := kubeClient.Get(ctx, key, &mutatedProwJob); err != nil {
		t.Fatalf("could not get prowjob: %v", err)
	}

	originalProwJobYAML, err := os.ReadFile(absValidBareProwJobPath)
	if err != nil {
		t.Fatalf("unable to read yaml file %v", err)
	}
	var originalProwJob v1.ProwJob
	err = yaml.Unmarshal(originalProwJobYAML, &originalProwJob)
	if err != nil {
		t.Fatalf("unable to unmarshal into prowjob %v", err)
	}
	diff := cmp.Diff(originalProwJob.Spec.DecorationConfig, mutatedProwJob.Spec.DecorationConfig)
	if diff == "" {
		t.Errorf("prowjob was not mutated %v", err)
	}
}
