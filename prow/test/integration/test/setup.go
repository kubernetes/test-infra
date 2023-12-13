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

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"sync"
	"testing"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultNamespace = "default"
	testpodNamespace = "test-pods"
)

var (
	clusterContext = flag.String("cluster", "kind-kind-prow-integration", "The context of cluster to use for test")

	jobConfigMux      sync.Mutex
	prowComponentsMux sync.Mutex
)

func getClusterContext() string {
	return *clusterContext
}

func NewClients(configPath, clusterName string) (ctrlruntimeclient.Client, error) {
	cfg, err := NewRestConfig(configPath, clusterName)
	if err != nil {
		return nil, err
	}
	return ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
}

func NewRestConfig(configPath, clusterName string) (*rest.Config, error) {
	var loader clientcmd.ClientConfigLoader
	if configPath != "" {
		loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: configPath}
	} else {
		loader = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	overrides := clientcmd.ConfigOverrides{}
	// Override the cluster name if provided.
	if clusterName != "" {
		overrides.Context.Cluster = clusterName
		overrides.CurrentContext = clusterName
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loader, &overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed create rest config: %w", err)
	}

	return cfg, nil
}

func getPodLogs(clientset *kubernetes.Clientset, namespace, podName string, opts *coreapi.PodLogOptions) (string, error) {
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return "", fmt.Errorf("error in opening stream")
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("error in copy information from podLogs to buf")
	}
	str := buf.String()

	return str, nil
}

func refreshProwPods(client ctrlruntimeclient.Client, ctx context.Context, name string) error {
	prowComponentsMux.Lock()
	defer prowComponentsMux.Unlock()

	var pods coreapi.PodList
	labels, _ := labels.Parse("app = " + name)
	if err := client.List(ctx, &pods, &ctrlruntimeclient.ListOptions{LabelSelector: labels}); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if err := client.Delete(ctx, &pod); err != nil {
			return err
		}
	}
	return nil
}

// RandomString generates random string of 32 characters in length, and fail if it failed
func RandomString(t *testing.T) string {
	b := make([]byte, 512)
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("failed to generate random: %v", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b[:]))[:32]
}

func updateJobConfig(ctx context.Context, kubeClient ctrlruntimeclient.Client, filename string, rawConfig []byte) error {
	jobConfigMux.Lock()
	defer jobConfigMux.Unlock()

	var existingMap coreapi.ConfigMap
	if err := kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{
		Namespace: defaultNamespace,
		Name:      "job-config",
	}, &existingMap); err != nil {
		return err
	}

	if existingMap.BinaryData == nil {
		existingMap.BinaryData = make(map[string][]byte)
	}
	existingMap.BinaryData[filename] = rawConfig
	return kubeClient.Update(ctx, &existingMap)
}

// execRemoteCommand is the Golang-equivalent of "kubectl exec". The command
// string should be something like {"/bin/sh", "-c", "..."} if you want to run a
// shell script.
//
// Adapted from https://discuss.kubernetes.io/t/go-client-exec-ing-a-shel-command-in-pod/5354/5.
func execRemoteCommand(restCfg *rest.Config, clientset *kubernetes.Clientset, pod *coreapi.Pod, command []string) (string, string, error) {
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	request := clientset.CoreV1().RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&coreapi.PodExecOptions{
			Command: command,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(restCfg, "POST", request.URL())
	if err != nil {
		return "", "", err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})
	if err != nil {
		return "", "", fmt.Errorf("%w Failed executing command %s on %v/%v", err, command, pod.Namespace, pod.Name)
	}

	// Return stdout, stderr.
	return buf.String(), errBuf.String(), nil
}
