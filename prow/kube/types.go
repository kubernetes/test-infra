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
	HostNetwork        bool              `json:"hostNetwork,omitempty"`
	Volumes            []Volume          `json:"volumes,omitempty"`
	InitContainers     []Container       `json:"initContainers,omitempty"`
	Containers         []Container       `json:"containers,omitempty"`
	RestartPolicy      string            `json:"restartPolicy,omitempty"`
	ServiceAccountName string            `json:"serviceAccountName,omitempty"`
	Tolerations        []Toleration      `json:"tolerations,omitempty"`
	NodeSelector       map[string]string `json:"nodeSelector,omitempty"`
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
	Name         string `json:"name,omitempty"`
	VolumeSource `json:",inline"`
}

// VolumeSource represents the source location of a volume to mount.
// Only one of its members may be specified.
type VolumeSource struct {
	HostPath    *HostPathSource       `json:"hostPath,omitempty"`
	EmptyDir    *EmptyDirVolumeSource `json:"emptyDir,omitemtpy"`
	Secret      *SecretSource         `json:"secret,omitempty"`
	DownwardAPI *DownwardAPISource    `json:"downwardAPI,omitempty"`
	ConfigMap   *ConfigMapSource      `json:"configMap,omitempty"`
}

type HostPathSource struct {
	Path string `json:"path,omitempty"`
}

type SecretSource struct {
	Name        string `json:"secretName,omitempty"`
	DefaultMode int32  `json:"defaultMode,omitempty"`
}

type EmptyDirVolumeSource struct {
	// NOTE: fields omitted here as Prow does not currently use them
}

type ConfigMapSource struct {
	Name string `json:"name,omitempty"`
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
	Name            string   `json:"name,omitempty"`
	Image           string   `json:"image,omitempty"`
	ImagePullPolicy string   `json:"imagePullPolicy,omitempty"`
	Command         []string `json:"command,omitempty"`
	Args            []string `json:"args,omitempty"`
	WorkDir         string   `json:"workingDir,omitempty"`
	Env             []EnvVar `json:"env,omitempty"`
	Ports           []Port   `json:"ports,omitempty"`

	Resources       Resources        `json:"resources,omitempty"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty"`
	VolumeMounts    []VolumeMount    `json:"volumeMounts,omitempty"`
	Lifecycle       *Lifecycle       `json:"lifecycle,omitempty"`
}

// Lifecycle describes actions that the management system should take in response to container lifecycle
// events. For the PostStart and PreStop lifecycle handlers, management of the container blocks
// until the action is complete, unless the container process fails, in which case the handler is aborted.
type Lifecycle struct {
	PostStart *Handler `json:"postStart,omitempty"`
	PreStop   *Handler `json:"preStop,omitempty"`
}

// Handler defines a specific action that should be taken
// TODO: pass structured data to these actions, and document that data here.
type Handler struct {
	// Exec specifies the action to take.
	Exec *ExecAction `json:"exec,omitempty"`
}

// ExecAction describes a "run in container" action.
type ExecAction struct {
	Command []string `json:"command,omitempty"`
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

type Toleration struct {
	Key               string             `json:"key,omitempty"`
	Operator          TolerationOperator `json:"operator,omitempty"`
	Value             string             `json:"value,omitempty"`
	Effect            TaintEffect        `json:"effect,omitempty"`
	TolerationSeconds *int64             `json:"tolerationSeconds,omitempty"`
}

type TaintEffect string

type TolerationOperator string
