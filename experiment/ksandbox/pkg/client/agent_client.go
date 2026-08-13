/*
Copyright 2025 The Kubernetes Authors.

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

package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os/exec"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	protocol "k8s.io/test-infra/experiment/ksandbox/protocol/ksandbox/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ClientCertificateDNSName is the expected CommonName to use in the client certificate
const ClientCertificateDNSName = "ksandbox-client"

// ServerCertificateDNSName is the expected DNSName to use in the server certificate
const ServerCertificateDNSName = "ksandbox-server"

// AgentClient manages running a Pod running the ksandbox-agent
type AgentClient struct {
	protocol.AgentClient

	conn        *grpc.ClientConn
	pod         *corev1.Pod
	secret      *corev1.Secret
	portForward *exec.Cmd

	k8sClient  kubernetes.Interface
	agentImage string

	caCert      *x509.Certificate
	caCertBytes []byte
	caKey       *ecdsa.PrivateKey

	serverCertBytes []byte
	serverKey       *ecdsa.PrivateKey

	clientCertBytes []byte
	clientKey       *ecdsa.PrivateKey
}

// NewAgentClient constructs a new AgentClient
func NewAgentClient(ctx context.Context, namespace string, agentImage string, image string, usePortForward bool) (*AgentClient, error) {
	if agentImage == "" {
		return nil, fmt.Errorf("agentImage is required")
	}

	if image == "" {
		return nil, fmt.Errorf("image is required")
	}

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes configuration: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	c := &AgentClient{
		k8sClient:  k8sClient,
		agentImage: agentImage,
	}

	if err := c.buildTLS(); err != nil {
		return nil, err
	}

	if err := c.startPod(ctx, namespace, image, usePortForward); err != nil {
		c.Close() // best effort

		return nil, err
	}

	return c, nil
}

func (c *AgentClient) Close() error {
	var ret error

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			ret = fmt.Errorf("failed to close grpc connection: %w", err)
		}
		c.conn = nil
	}

	if c.portForward != nil {
		if err := c.portForward.Process.Kill(); err != nil {
			ret = fmt.Errorf("failed to kill port-forward process: %w", err)
		}
		c.portForward = nil
	}

	if c.secret != nil {
		err := c.k8sClient.CoreV1().Secrets(c.secret.Namespace).Delete(context.Background(), c.secret.Name, metav1.DeleteOptions{})
		if err != nil {
			ret = fmt.Errorf("failed to clean up secret: %w", err)
		}
		c.secret = nil
	}

	if err := c.stopPod(context.Background()); err != nil {
		ret = err
	}

	return ret
}

// buildLabels creates the labels we should apply to our resources
func (c *AgentClient) buildLabels() map[string]string {
	labels := map[string]string{
		"ksandbox-agent": "1",
	}
	return labels
}

// buildSecret creates the Secret object for creation
func (c *AgentClient) buildSecret(namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secret.GenerateName = "agent-"
	secret.Namespace = namespace
	secret.Labels = c.buildLabels()
	secret.Data = map[string][]byte{}

	var err error
	secret.Data["server.crt"], err = certToPEM(c.serverCertBytes)
	if err != nil {
		return nil, err
	}
	secret.Data["server.key"], err = keyToPEM(c.serverKey)
	if err != nil {
		return nil, err
	}
	secret.Data["client-ca.crt"], err = certToPEM(c.caCertBytes)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// buildPod creates the Pod object for creation
func (c *AgentClient) buildPod(namespace string, image string) (*corev1.Pod, error) {
	// TODO: randomize the mount path?
	mountPath := "/.agent"

	// TODO: Load this from a template?

	sharedVolume := corev1.Volume{}
	sharedVolume.Name = "agent"
	sharedVolume.EmptyDir = &corev1.EmptyDirVolumeSource{}

	tlsVolume := corev1.Volume{}
	tlsVolume.Name = "agent-tls"
	tlsVolume.Secret = &corev1.SecretVolumeSource{
		SecretName: c.secret.Name,
	}

	initContainer := corev1.Container{}
	initContainer.Name = "agent"
	initContainer.Image = c.agentImage
	initContainer.Args = []string{"--install", mountPath}
	initContainer.VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: mountPath,
			Name:      sharedVolume.Name,
		},
		{
			MountPath: "/tls",
			Name:      tlsVolume.Name,
		},
	}

	// TODO: Readiness check on the agent?

	container := corev1.Container{}
	container.Name = "main"
	container.Image = image
	container.Command = []string{
		path.Join(mountPath, "ksandbox-agent"),
		"--tls-dir=" + path.Join(mountPath, "tls"),
	}
	container.VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: mountPath,
			Name:      sharedVolume.Name,
		},
	}
	container.Resources.Requests = make(corev1.ResourceList)
	container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("4")
	container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("8Gi")
	container.Resources.Requests[corev1.ResourceEphemeralStorage] = resource.MustParse("10Gi")

	container.Resources.Limits = make(corev1.ResourceList)
	for k, v := range container.Resources.Requests {
		if k == corev1.ResourceCPU {
			continue // Allow bursting
		}
		container.Resources.Limits[k] = v
	}

	pod := &corev1.Pod{}
	pod.Namespace = namespace
	pod.GenerateName = "agent-test-"
	pod.Spec.InitContainers = []corev1.Container{initContainer}
	pod.Spec.Containers = []corev1.Container{container}
	pod.Spec.Volumes = []corev1.Volume{sharedVolume, tlsVolume}
	pod.Labels = c.buildLabels()
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	automountServiceAccountToken := false
	pod.Spec.AutomountServiceAccountToken = &automountServiceAccountToken

	return pod, nil
}

func (c *AgentClient) startPod(ctx context.Context, namespace string, image string, usePortForward bool) error {

	{
		secret, err := c.buildSecret(namespace)
		if err != nil {
			return err
		}

		s, err := c.k8sClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("error creating secret: %w", err)
		}
		c.secret = s
	}

	pod, err := c.buildPod(namespace, image)
	if err != nil {
		return err
	}

	p, err := c.k8sClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating pod: %w", err)
	}
	c.pod = p

	// TODO: Make timeout configurable?
	timeout := 5 * time.Minute // We want to allow extra time in case we have to scale up the cluster
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pod, err = c.waitForPodReady(ctxWithTimeout)
	if err != nil {
		return err
	}
	// TODO: wait for podIP?

	port := "7007"
	targetIP := pod.Status.PodIP

	// If we're running locally (debug mode), we start a port-forward process to tunnel to the pod
	// Normally this wouldn't be needed
	if usePortForward {
		portForward := exec.Command("kubectl", "port-forward", c.pod.Name, "-n", c.pod.Namespace, port)
		if err := portForward.Start(); err != nil {
			return fmt.Errorf("error starting port-forward")
		}
		klog.Infof("starting port-forward (DEBUG MODE): %v", portForward)
		c.portForward = portForward
		targetIP = "127.0.0.1"

		// Allow for the port-forward to start
		time.Sleep(2 * time.Second)
	}

	serverCertPool := x509.NewCertPool()
	serverCertPool.AddCert(c.caCert)

	clientKeypair := tls.Certificate{
		Certificate: [][]byte{c.clientCertBytes},
		PrivateKey:  c.clientKey,
	}

	tlsConfig := &tls.Config{
		ServerName:   ServerCertificateDNSName,
		Certificates: []tls.Certificate{clientKeypair},
		RootCAs:      serverCertPool,
	}

	klog.Infof("dialing server %s", targetIP)

	conn, err := grpc.DialContext(ctx, net.JoinHostPort(targetIP, port),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return fmt.Errorf("failed to dial agent GRPC service: %w", err)
	}

	c.conn = conn
	c.AgentClient = protocol.NewAgentClient(conn)
	return nil
}

func (c *AgentClient) stopPod(ctx context.Context) error {
	if c.pod == nil {
		return nil
	}

	err := c.k8sClient.CoreV1().Pods(c.pod.Namespace).Delete(ctx, c.pod.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}
	c.pod = nil
	return nil
}

// waitForPodReady polls the pod until it is ready, so we can connect to the agent
func (c *AgentClient) waitForPodReady(ctx context.Context) (*corev1.Pod, error) {
	log := klog.FromContext(ctx)

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		pod, err := c.k8sClient.CoreV1().Pods(c.pod.Namespace).Get(ctx, c.pod.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting pod: %w", err)
		}
		ready := false
		switch pod.Status.Phase {
		case corev1.PodRunning:
			ready = true

		case corev1.PodPending:
			// not ready

		case corev1.PodFailed:
			log.Info("pod entered failed state", "pod", pod)
			return nil, fmt.Errorf("pod entered failed state")

		default:
			klog.Warningf("unknown pod status %q", pod.Status.Phase)
		}

		if ready {
			for _, status := range pod.Status.ContainerStatuses {
				if !status.Ready {
					ready = false
				}
			}
		}

		if ready {
			return pod, nil
		}

		time.Sleep(1 * time.Second)
	}
}

// Pod returns the pod, useful for constructing references
func (c *AgentClient) Pod() *corev1.Pod {
	return c.pod
}
