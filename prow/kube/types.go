/*
Copyright 2016 The Kubernetes Authors.

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

package kube

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectMeta = metav1.ObjectMeta

type Pod = corev1.Pod
type PodSpec = corev1.PodSpec
type PodStatus = corev1.PodStatus

const (
	PodPending   = corev1.PodPending
	PodRunning   = corev1.PodRunning
	PodSucceeded = corev1.PodSucceeded
	PodFailed    = corev1.PodFailed
	PodUnknown   = corev1.PodUnknown
)

const (
	Evicted = "Evicted"
)

type Container = corev1.Container

type EnvVar = corev1.EnvVar

type ConfigMap = corev1.ConfigMap

func MetaTime(t time.Time) *metav1.Time {
	metaTime := metav1.NewTime(t)
	return &metaTime
}
