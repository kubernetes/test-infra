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
)

type ObjectMeta struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	ResourceVersion string `json:"resourceVersion,omitempty"`
	UID             string `json:"uid,omitempty"`
}

type Secret struct {
	Metadata ObjectMeta        `json:"metadata,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}

type Job struct {
	Metadata ObjectMeta `json:"metadata,omitempty"`
	Spec     JobSpec    `json:"spec,omitempty"`
	Status   JobStatus  `json:"status,omitempty"`
}

func (j *Job) Complete() bool {
	if j.Status.Succeeded > 0 {
		return true
	} else if j.Status.Active > 0 {
		return false
	} else if j.Spec.Parallelism != nil && *j.Spec.Parallelism == 0 {
		return true
	}
	return false
}

type JobSpec struct {
	Completions           *int `json:"completions,omitempty"`
	Parallelism           *int `json:"parallelism,omitempty"`
	ActiveDeadlineSeconds int  `json:"activeDeadlineSeconds,omitempty"`

	Template PodTemplateSpec `json:"template,omitempty"`
}

type JobStatus struct {
	StartTime      time.Time `json:"startTime,omitempty"`
	CompletionTime time.Time `json:"completionTime,omitempty"`
	Active         int       `json:"active,omitempty"`
	Succeeded      int       `json:"succeeded,omitempty"`
	Failed         int       `json:"failed,omitempty"`
}

type PodTemplateSpec struct {
	Metadata ObjectMeta `json:"metadata,omitempty"`
	Spec     PodSpec    `json:"spec,omitempty"`
}

type Pod struct {
	Metadata ObjectMeta `json:"metadata,omitempty"`
	Spec     PodSpec    `json:"spec,omitempty"`
	Status   PodStatus  `json:"status,omitempty"`
}

type PodSpec struct {
	Volumes       []Volume          `json:"volumes,omitempty"`
	Containers    []Container       `json:"containers,omitempty"`
	RestartPolicy string            `json:"restartPolicy,omitempty"`
	NodeSelector  map[string]string `json:"nodeSelector,omitempty"`
}

type PodPhase string

const (
	PodPending   PodPhase = "Pending"
	PodRunning   PodPhase = "Running"
	PodSucceeded PodPhase = "Succeeded"
	PodFailed    PodPhase = "Failed"
	PodUnknown   PodPhase = "Unknown"
)

const (
	Evicted = "Evicted"
)

type PodStatus struct {
	Phase     PodPhase  `json:"phase,omitempty"`
	Message   string    `json:"message,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	StartTime time.Time `json:"startTime,omitempty"`
}

type Volume struct {
	Name        string             `json:"name,omitempty"`
	Secret      *SecretSource      `json:"secret,omitempty"`
	DownwardAPI *DownwardAPISource `json:"downwardAPI,omitempty"`
	HostPath    *HostPathSource    `json:"hostPath,omitempty"`
	ConfigMap   *ConfigMapSource   `json:"configMap,omitempty"`
}

type ConfigMapSource struct {
	Name string `json:"name,omitempty"`
}

type HostPathSource struct {
	Path string `json:"path,omitempty"`
}

type SecretSource struct {
	Name        string `json:"secretName,omitempty"`
	DefaultMode int32  `json:"defaultMode,omitempty"`
}

type DownwardAPISource struct {
	Items []DownwardAPIFile `json:"items,omitempty"`
}

type DownwardAPIFile struct {
	Path  string              `json:"path"`
	Field ObjectFieldSelector `json:"fieldRef,omitempty"`
}

type ObjectFieldSelector struct {
	FieldPath string `json:"fieldPath"`
}

type Container struct {
	Name    string   `json:"name,omitempty"`
	Image   string   `json:"image,omitempty"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"workingDir,omitempty"`
	Env     []EnvVar `json:"env,omitempty"`
	Ports   []Port   `json:"ports,omitempty"`

	Resources       Resources        `json:"resources,omitempty"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty"`
	VolumeMounts    []VolumeMount    `json:"volumeMounts,omitempty"`
}

type Port struct {
	ContainerPort int `json:"containerPort,omitempty"`
	HostPort      int `json:"hostPort,omitempty"`
}

type EnvVar struct {
	Name      string        `json:"name,omitempty"`
	Value     string        `json:"value,omitempty"`
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

type EnvVarSource struct {
	ConfigMap ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

type ConfigMapKeySelector struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type Resources struct {
	Requests *ResourceRequest `json:"requests,omitempty"`
	Limits   *ResourceRequest `json:"limits,omitempty"`
}

type ResourceRequest struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type SecurityContext struct {
	Privileged bool `json:"privileged,omitempty"`
}

type VolumeMount struct {
	Name      string `json:"name,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

type ConfigMap struct {
	Metadata ObjectMeta        `json:"metadata,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}
