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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: Drop all of these

type ObjectMeta = metav1.ObjectMeta

type Pod = v1.Pod
type PodTemplateSpec = v1.PodTemplateSpec
type PodSpec = v1.PodSpec
type PodStatus = v1.PodStatus

const (
	PodPending   = v1.PodPending
	PodRunning   = v1.PodRunning
	PodSucceeded = v1.PodSucceeded
	PodFailed    = v1.PodFailed
	PodUnknown   = v1.PodUnknown
)

const (
	Evicted = "Evicted"
)

type Container = v1.Container
type Port = v1.ContainerPort
type EnvVar = v1.EnvVar

type Volume = v1.Volume
type VolumeMount = v1.VolumeMount
type VolumeSource = v1.VolumeSource
type EmptyDirVolumeSource = v1.EmptyDirVolumeSource
type SecretSource = v1.SecretVolumeSource
type ConfigMapSource = v1.ConfigMapVolumeSource

type ConfigMap = v1.ConfigMap
type Secret = v1.Secret
