/*
Copyright 2018 The Kubernetes Authors.

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

package decorate

import (
	"fmt"
	"sort"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/prow/clonerefs"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/initupload"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
	"k8s.io/test-infra/prow/sidecar"
)

const (
	LogMountName            = "logs"
	LogMountPath            = "/logs"
	ArtifactsEnv            = "ARTIFACTS"
	ArtifactsPath           = LogMountPath + "/artifacts"
	CodeMountName           = "code"
	CodeMountPath           = "/home/prow/go"
	ToolsMountName          = "tools"
	ToolsMountPath          = "/tools"
	GcsCredentialsMountName = "gcs-credentials"
	GcsCredentialsMountPath = "/secrets/gcs"
)

func Labels() []string {
	return []string{kube.ProwJobTypeLabel, kube.CreatedByProw}
}

func VolumeMounts() []string {
	return []string{LogMountName, CodeMountName, ToolsMountName, GcsCredentialsMountName}
}

func VolumeMountPaths() []string {
	return []string{LogMountPath, CodeMountPath, ToolsMountPath, GcsCredentialsMountPath}
}

// ProwJobToPod converts a ProwJob to a Pod that will run the tests.
func ProwJobToPod(pj kube.ProwJob, buildID string) (*v1.Pod, error) {
	if pj.Spec.PodSpec == nil {
		return nil, fmt.Errorf("prowjob %q lacks a pod spec", pj.Name)
	}

	rawEnv, err := downwardapi.EnvForSpec(downwardapi.NewJobSpec(pj.Spec, buildID, pj.Name))
	if err != nil {
		return nil, err
	}
	env := kubeEnv(rawEnv)

	spec := pj.Spec.PodSpec.DeepCopy()
	spec.RestartPolicy = "Never"
	spec.Containers[0].Name = "test"

	if pj.Spec.DecorationConfig == nil {
		spec.Containers[0].Env = append(spec.Containers[0].Env, env...)
	} else {
		rawEnv[ArtifactsEnv] = ArtifactsPath
		logMount := kube.VolumeMount{
			Name:      LogMountName,
			MountPath: LogMountPath,
		}
		logVolume := kube.Volume{
			Name: LogMountName,
			VolumeSource: kube.VolumeSource{
				EmptyDir: &kube.EmptyDirVolumeSource{},
			},
		}

		codeMount := kube.VolumeMount{
			Name:      CodeMountName,
			MountPath: CodeMountPath,
		}
		codeVolume := kube.Volume{
			Name: CodeMountName,
			VolumeSource: kube.VolumeSource{
				EmptyDir: &kube.EmptyDirVolumeSource{},
			},
		}

		toolsMount := kube.VolumeMount{
			Name:      ToolsMountName,
			MountPath: ToolsMountPath,
		}
		toolsVolume := kube.Volume{
			Name: ToolsMountName,
			VolumeSource: kube.VolumeSource{
				EmptyDir: &kube.EmptyDirVolumeSource{},
			},
		}

		gcsCredentialsMount := kube.VolumeMount{
			Name:      GcsCredentialsMountName,
			MountPath: GcsCredentialsMountPath,
		}
		gcsCredentialsVolume := kube.Volume{
			Name: GcsCredentialsMountName,
			VolumeSource: kube.VolumeSource{
				Secret: &kube.SecretSource{
					SecretName: pj.Spec.DecorationConfig.GCSCredentialsSecret,
				},
			},
		}

		cloneLog := fmt.Sprintf("%s/clone.json", LogMountPath)
		var refs []*kube.Refs
		refs = append(refs, pj.Spec.Refs)
		refs = append(refs, pj.Spec.ExtraRefs...)
		cloneConfigEnv, err := clonerefs.Encode(clonerefs.Options{
			SrcRoot:      CodeMountPath,
			Log:          cloneLog,
			GitUserName:  clonerefs.DefaultGitUserName,
			GitUserEmail: clonerefs.DefaultGitUserEmail,
			GitRefs:      refs,
		})
		if err != nil {
			return nil, fmt.Errorf("could not encode clone configuration as JSON: %v", err)
		}

		gcsOptions := gcsupload.Options{
			// TODO: pass the artifact dir here too once we figure that out
			GCSConfiguration:   pj.Spec.DecorationConfig.GCSConfiguration,
			GcsCredentialsFile: fmt.Sprintf("%s/service-account.json", GcsCredentialsMountPath),
			DryRun:             false,
		}
		initUploadConfigEnv, err := initupload.Encode(initupload.Options{
			Log:     cloneLog,
			Options: &gcsOptions,
		})
		if err != nil {
			return nil, fmt.Errorf("could not encode initupload configuration as JSON: %v", err)
		}

		entrypointLocation := fmt.Sprintf("%s/entrypoint", ToolsMountPath)

		spec.InitContainers = []kube.Container{
			{
				Name:         "clonerefs",
				Image:        pj.Spec.DecorationConfig.UtilityImages.CloneRefs,
				Command:      []string{"/clonerefs"},
				Env:          kubeEnv(map[string]string{clonerefs.JSONConfigEnvVar: cloneConfigEnv}),
				VolumeMounts: []kube.VolumeMount{logMount, codeMount},
			},
			{
				Name:    "initupload",
				Image:   pj.Spec.DecorationConfig.UtilityImages.InitUpload,
				Command: []string{"/initupload"},
				Env: kubeEnv(map[string]string{
					initupload.JSONConfigEnvVar: initUploadConfigEnv,
					downwardapi.JobSpecEnv:      rawEnv[downwardapi.JobSpecEnv], // TODO: shouldn't need this?
				}),
				VolumeMounts: []kube.VolumeMount{logMount, gcsCredentialsMount},
			},
			{
				Name:         "place-tools",
				Image:        pj.Spec.DecorationConfig.UtilityImages.Entrypoint,
				Command:      []string{"/bin/cp"},
				Args:         []string{"/entrypoint", entrypointLocation},
				VolumeMounts: []kube.VolumeMount{toolsMount},
			},
		}

		wrapperOptions := wrapper.Options{
			ProcessLog: fmt.Sprintf("%s/process-log.txt", LogMountPath),
			MarkerFile: fmt.Sprintf("%s/marker-file.txt", LogMountPath),
		}
		entrypointConfigEnv, err := entrypoint.Encode(entrypoint.Options{
			Args:        append(spec.Containers[0].Command, spec.Containers[0].Args...),
			Options:     &wrapperOptions,
			Timeout:     pj.Spec.DecorationConfig.Timeout,
			GracePeriod: pj.Spec.DecorationConfig.GracePeriod,
		})
		if err != nil {
			return nil, fmt.Errorf("could not encode entrypoint configuration as JSON: %v", err)
		}
		allEnv := rawEnv
		allEnv[entrypoint.JSONConfigEnvVar] = entrypointConfigEnv

		spec.Containers[0].Command = []string{entrypointLocation}
		if pj.Spec.Type != kube.PeriodicJob {
			spec.Containers[0].WorkingDir = clone.PathForRefs(CodeMountPath, pj.Spec.Refs)
		}
		spec.Containers[0].Args = []string{}
		spec.Containers[0].Env = append(spec.Containers[0].Env, kubeEnv(allEnv)...)
		spec.Containers[0].VolumeMounts = append(spec.Containers[0].VolumeMounts, logMount, codeMount, toolsMount)

		sidecarConfigEnv, err := sidecar.Encode(sidecar.Options{
			GcsOptions:     &gcsOptions,
			WrapperOptions: &wrapperOptions,
		})
		if err != nil {
			return nil, fmt.Errorf("could not encode sidecar configuration as JSON: %v", err)
		}

		spec.Containers = append(spec.Containers, kube.Container{
			Name:    "sidecar",
			Image:   pj.Spec.DecorationConfig.UtilityImages.Sidecar,
			Command: []string{"/sidecar"},
			Env: kubeEnv(map[string]string{
				sidecar.JSONConfigEnvVar: sidecarConfigEnv,
				downwardapi.JobSpecEnv:   rawEnv[downwardapi.JobSpecEnv], // TODO: shouldn't need this?
			}),
			VolumeMounts: []kube.VolumeMount{logMount, gcsCredentialsMount},
		})
		spec.Volumes = append(spec.Volumes, logVolume, codeVolume, toolsVolume, gcsCredentialsVolume)
	}

	podLabels := make(map[string]string)
	for k, v := range pj.ObjectMeta.Labels {
		podLabels[k] = v
	}
	podLabels[kube.CreatedByProw] = "true"
	podLabels[kube.ProwJobTypeLabel] = string(pj.Spec.Type)
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pj.ObjectMeta.Name,
			Labels: podLabels,
			Annotations: map[string]string{
				kube.ProwJobAnnotation: pj.Spec.Job,
			},
		},
		Spec: *spec,
	}, nil
}

// kubeEnv transforms a mapping of environment variables
// into their serialized form for a PodSpec, sorting by
// the name of the env vars
func kubeEnv(environment map[string]string) []v1.EnvVar {
	var keys []string
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var kubeEnvironment []v1.EnvVar
	for _, key := range keys {
		kubeEnvironment = append(kubeEnvironment, v1.EnvVar{
			Name:  key,
			Value: environment[key],
		})
	}

	return kubeEnvironment
}
