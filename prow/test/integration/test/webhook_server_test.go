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
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	validProwJobID         = "test-job2"
	validBareProwJobID     = "test-mutate-job"
	validBareProwJobPath   = "./testdata/valid_bare_prowjob.yaml"
	invalidProwJobPath     = "./testdata/invalid_prowjob.yaml"
	validProwJobPath       = "./testdata/valid_prowjob.yaml"
	deploymentPath         = "../config/prow/cluster/webhook_server_deployment.yaml"
	testServerAvailability = "test-server-availability"
)

func TestWebhookServerValidateCluster(t *testing.T) {
	t.Cleanup(func() {
		err := cleanUpCluster(validProwJobID)
		if err != nil {
			t.Logf("could not delete prowjob %s", validProwJobID)
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
			t.Logf("could not delete prowjob %s", validProwJobID)
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
	originalProwJobYAML, err := ioutil.ReadFile(absValidBareProwJobPath)
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

func TestWebhookServerAvailability(t *testing.T) {
	clusterContext := getClusterContext()
	kubeClient, err := NewClients("", clusterContext)
	if err != nil {
		t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
	}
	ctx := context.Background()
	t.Cleanup(func() {
		absDeploymentPath, err := filepath.Abs(deploymentPath)
		if err != nil {
			t.Logf("could not find absolute path")
		}
		out, err := exec.Command("kubectl", "--context="+getClusterContext(), "apply", "-f", absDeploymentPath).CombinedOutput()
		if err != nil {
			t.Logf("could not clean up cluster %s", string(out))
		}
		testAvailabilityKey := types.NamespacedName {
			Namespace: "default",
			Name: testServerAvailability,
		}
		mutatedProwJobKey := types.NamespacedName {
			Namespace: "default",
			Name: validBareProwJobID,
		}
		var testAvailabilityProwJob v1.ProwJob
		var mutatedProwJob v1.ProwJob
		if err := kubeClient.Get(ctx, testAvailabilityKey, &testAvailabilityProwJob); err != nil {
			t.Logf("could not get prowjob %v", err)
		}
		if err := kubeClient.Get(ctx, mutatedProwJobKey, &mutatedProwJob); err != nil {
			t.Logf("could not get prowjob %v", err)
		}
		if err := kubeClient.Delete(ctx, &testAvailabilityProwJob); err != nil {
			t.Logf("could not delete prowjob %v", err)
		}
		if err := kubeClient.Delete(ctx, &mutatedProwJob); err != nil {
			t.Logf("could not delete prowjob %v", err)
		}
	})

	key := types.NamespacedName{
		Namespace: "default",
		Name:      "webhook-server",
	}
	var webhookDeployment appsv1.Deployment
	err = kubeClient.Get(ctx, key, &webhookDeployment)
	if err != nil {
		t.Fatalf("could not get deployment %v", err)
	}
	originalProwJob := &v1.ProwJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ProwJob",
			APIVersion: "prow.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"foo":               "bar",
				"default-me":        "enabled",
				"admission-webhook": "enabled",
			},
			Name:      testServerAvailability,
			Namespace: "default",
		},
		Spec: v1.ProwJobSpec{
			Cluster: "foo",
			Refs: &v1.Refs{
				Repo: "bar",
			},
			Agent:  v1.TektonAgent,
			Type:   v1.PresubmitJob,
			Report: true,
		},
		Status: v1.ProwJobStatus{
			State: v1.PendingState,
		},
	}
	selector, err := labels.Parse("app = webhook-server")
	if err != nil {
		t.Fatal("could not parse labels")
	}
	webhookDeploymentCopy := webhookDeployment.DeepCopy()
	webhookDeploymentCopy.Spec.Template.Spec.Containers[0].LivenessProbe.PeriodSeconds = 4
	var podList corev1.PodList
	if err := kubeClient.List(ctx, &podList, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		t.Fatalf("could not list pods %v", err)
	}
	go func() {
		if err := kubeClient.Patch(ctx, webhookDeploymentCopy, client.MergeFrom(&webhookDeployment), &client.PatchOptions{}); err != nil {
			t.Errorf("could not patch deployment %v", err)
		}
	}()
	oldNames := sets.NewString()
	for _, pod := range podList.Items {
		oldNames.Insert(pod.Name)
	}
	for {
		exist, newPod, err := checkNewPodInCluster(oldNames, kubeClient, ctx)
		if err != nil {
			t.Fatalf("could not check for new pod in cluster %v", err)
		}
		var found bool
		var podList corev1.PodList
		kubeClient.List(ctx, &podList)
		for _, pod := range podList.Items {
			if oldNames.Has(pod.Name) {
				found = true
			}
		}
		if exist && newPod.Status.Phase == "Running" && !found {
			return
		}
		//needed in order to create a prowjob without a resource version.
		originalProwJobCopy := originalProwJob.DeepCopy()
		if err := kubeClient.Create(ctx, originalProwJobCopy, &client.CreateOptions{}); err != nil {
			t.Fatalf("prowjob could not be created. Error: %v ", err)
		}
		out, err := exec.Command("kubectl", "--context="+getClusterContext(), "delete", "prowjob", testServerAvailability).CombinedOutput()
		if err != nil {
			t.Fatalf("could not delete prowjob. Error: %v Output: %s", err, string(out))
		}
	}
	//one more call to mutate the prowjob to check the newPod is working as expected
}

func checkNewPodInCluster(originalNames sets.String, kubeClient client.Client, ctx context.Context) (bool, corev1.Pod, error) {
	var targetPod corev1.Pod
	var podList corev1.PodList
	err := kubeClient.List(ctx, &podList, &client.ListOptions{})
	if err != nil {
		return false, targetPod, fmt.Errorf("could not list pods %v", err)
	}
	for _, pod := range podList.Items {
		if pod.ObjectMeta.Labels["app"] == "webhook-server" && !originalNames.Has(pod.Name) {
			return true, pod, nil
		}
	}
	return false, targetPod, nil
}

