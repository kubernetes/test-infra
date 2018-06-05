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
	"testing"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"

	"k8s.io/test-infra/prow/kube"
)

func TestProwJobToPod(t *testing.T) {
	var sshKeyMode int32 = 0400
	tests := []struct {
		podName string
		buildID string
		labels  map[string]string
		pjSpec  kube.ProwJobSpec

		expected *v1.Pod
	}{
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type:  kube.PresubmitJob,
				Job:   "job-name",
				Agent: kube.KubernetesAgent,
				Refs: &kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
				},
				PodSpec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "tester",
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},

			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "presubmit",
						kube.ProwJobIDLabel:   "pod",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					Containers: []v1.Container{
						{
							Name:  "test",
							Image: "tester",
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}]}}`},
								{Name: "JOB_TYPE", Value: "presubmit"},
								{Name: "PROW_JOB_ID", Value: "pod"},
								{Name: "PULL_BASE_REF", Value: "base-ref"},
								{Name: "PULL_BASE_SHA", Value: "base-sha"},
								{Name: "PULL_NUMBER", Value: "1"},
								{Name: "PULL_PULL_SHA", Value: "pull-sha"},
								{Name: "PULL_REFS", Value: "base-ref:base-sha,1:pull-sha"},
								{Name: "REPO_NAME", Value: "repo-name"},
								{Name: "REPO_OWNER", Value: "org-name"},
							},
						},
					},
				},
			},
		},
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &kube.DecorationConfig{
					Timeout:     120 * time.Minute,
					GracePeriod: 10 * time.Second,
					UtilityImages: &kube.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &kube.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: "secret-name",
					SshKeySecrets:        []string{"ssh-1", "ssh-2"},
				},
				Agent: kube.KubernetesAgent,
				Refs: &kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []*kube.Refs{},
				PodSpec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "presubmit",
						kube.ProwJobIDLabel:   "pod",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					InitContainers: []v1.Container{
						{
							Name:    "clonerefs",
							Image:   "clonerefs:tag",
							Command: []string{"/clonerefs"},
							Env: []v1.EnvVar{
								{Name: "CLONEREFS_OPTIONS", Value: `{"src_root":"/home/prow/go","log":"/logs/clone.json","git_user_name":"ci-robot","git_user_email":"ci-robot@k8s.io","refs":[{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}],"key_files":["/secrets/ssh/ssh-1","/secrets/ssh/ssh-2"]}`},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "code",
									MountPath: "/home/prow/go",
								},
								{
									Name:      "ssh-keys-ssh-1",
									MountPath: "/secrets/ssh/ssh-1",
									ReadOnly:  true,
								},
								{
									Name:      "ssh-keys-ssh-2",
									MountPath: "/secrets/ssh/ssh-2",
									ReadOnly:  true,
								},
							},
						},
						{
							Name:    "initupload",
							Image:   "initupload:tag",
							Command: []string{"/initupload"},
							Env: []v1.EnvVar{
								{Name: "INITUPLOAD_OPTIONS", Value: `{"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false,"log":"/logs/clone.json"}`},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
						{
							Name:    "place-tools",
							Image:   "entrypoint:tag",
							Command: []string{"/bin/cp"},
							Args: []string{
								"/entrypoint",
								"/tools/entrypoint",
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "tools",
									MountPath: "/tools",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:       "test",
							Image:      "tester",
							Command:    []string{"/tools/entrypoint"},
							Args:       []string{},
							WorkingDir: "/home/prow/go/src/somewhere/else",
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "ARTIFACTS", Value: "/logs/artifacts"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "ENTRYPOINT_OPTIONS", Value: `{"args":["/bin/thing","some","args"],"timeout":7200000000000,"grace_period":10000000000,"artifact_dir":"/logs/artifacts","process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}`},
								{Name: "GOPATH", Value: "/home/prow/go"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
								{Name: "JOB_TYPE", Value: "presubmit"},
								{Name: "PROW_JOB_ID", Value: "pod"},
								{Name: "PULL_BASE_REF", Value: "base-ref"},
								{Name: "PULL_BASE_SHA", Value: "base-sha"},
								{Name: "PULL_NUMBER", Value: "1"},
								{Name: "PULL_PULL_SHA", Value: "pull-sha"},
								{Name: "PULL_REFS", Value: "base-ref:base-sha,1:pull-sha"},
								{Name: "REPO_NAME", Value: "repo-name"},
								{Name: "REPO_OWNER", Value: "org-name"},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "tools",
									MountPath: "/tools",
								},
								{
									Name:      "code",
									MountPath: "/home/prow/go",
								},
							},
						},
						{
							Name:    "sidecar",
							Image:   "sidecar:tag",
							Command: []string{"/sidecar"},
							Env: []v1.EnvVar{
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
								{Name: "SIDECAR_OPTIONS", Value: `{"gcs_options":{"items":["/logs/artifacts"],"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false},"wrapper_options":{"process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}}`},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "logs",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "tools",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "gcs-credentials",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "secret-name",
								},
							},
						},
						{
							Name: "ssh-keys-ssh-1",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName:  "ssh-1",
									DefaultMode: &sshKeyMode,
								},
							},
						},
						{
							Name: "ssh-keys-ssh-2",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName:  "ssh-2",
									DefaultMode: &sshKeyMode,
								},
							},
						},
						{
							Name: "code",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "job-name",
				DecorationConfig: &kube.DecorationConfig{
					Timeout:     120 * time.Minute,
					GracePeriod: 10 * time.Second,
					UtilityImages: &kube.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &kube.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: "secret-name",
					SshKeySecrets:        []string{"ssh-1", "ssh-2"},
				},
				Agent: kube.KubernetesAgent,
				PodSpec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "periodic",
						kube.ProwJobIDLabel:   "pod",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					InitContainers: []v1.Container{
						{
							Name:    "initupload",
							Image:   "initupload:tag",
							Command: []string{"/initupload"},
							Env: []v1.EnvVar{
								{Name: "INITUPLOAD_OPTIONS", Value: `{"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false}`},
								{Name: "JOB_SPEC", Value: `{"type":"periodic","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{}}`},
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
						{
							Name:    "place-tools",
							Image:   "entrypoint:tag",
							Command: []string{"/bin/cp"},
							Args: []string{
								"/entrypoint",
								"/tools/entrypoint",
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "tools",
									MountPath: "/tools",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:    "test",
							Image:   "tester",
							Command: []string{"/tools/entrypoint"},
							Args:    []string{},
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "ARTIFACTS", Value: "/logs/artifacts"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "ENTRYPOINT_OPTIONS", Value: `{"args":["/bin/thing","some","args"],"timeout":7200000000000,"grace_period":10000000000,"artifact_dir":"/logs/artifacts","process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}`},
								{Name: "GOPATH", Value: "/home/prow/go"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_SPEC", Value: `{"type":"periodic","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{}}`},
								{Name: "JOB_TYPE", Value: "periodic"},
								{Name: "PROW_JOB_ID", Value: "pod"},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "tools",
									MountPath: "/tools",
								},
							},
						},
						{
							Name:    "sidecar",
							Image:   "sidecar:tag",
							Command: []string{"/sidecar"},
							Env: []v1.EnvVar{
								{Name: "JOB_SPEC", Value: `{"type":"periodic","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{}}`},
								{Name: "SIDECAR_OPTIONS", Value: `{"gcs_options":{"items":["/logs/artifacts"],"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false},"wrapper_options":{"process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}}`},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "logs",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "tools",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "gcs-credentials",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "secret-name",
								},
							},
						},
					},
				},
			},
		},
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &kube.DecorationConfig{
					Timeout:     120 * time.Minute,
					GracePeriod: 10 * time.Second,
					UtilityImages: &kube.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &kube.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: "secret-name",
					SshKeySecrets:        []string{"ssh-1", "ssh-2"},
					SkipCloning:          true,
				},
				Agent: kube.KubernetesAgent,
				Refs: &kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []*kube.Refs{},
				PodSpec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "presubmit",
						kube.ProwJobIDLabel:   "pod",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					InitContainers: []v1.Container{
						{
							Name:    "initupload",
							Image:   "initupload:tag",
							Command: []string{"/initupload"},
							Env: []v1.EnvVar{
								{Name: "INITUPLOAD_OPTIONS", Value: `{"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false}`},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
						{
							Name:    "place-tools",
							Image:   "entrypoint:tag",
							Command: []string{"/bin/cp"},
							Args: []string{
								"/entrypoint",
								"/tools/entrypoint",
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "tools",
									MountPath: "/tools",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:    "test",
							Image:   "tester",
							Command: []string{"/tools/entrypoint"},
							Args:    []string{},
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "ARTIFACTS", Value: "/logs/artifacts"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "ENTRYPOINT_OPTIONS", Value: `{"args":["/bin/thing","some","args"],"timeout":7200000000000,"grace_period":10000000000,"artifact_dir":"/logs/artifacts","process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}`},
								{Name: "GOPATH", Value: "/home/prow/go"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
								{Name: "JOB_TYPE", Value: "presubmit"},
								{Name: "PROW_JOB_ID", Value: "pod"},
								{Name: "PULL_BASE_REF", Value: "base-ref"},
								{Name: "PULL_BASE_SHA", Value: "base-sha"},
								{Name: "PULL_NUMBER", Value: "1"},
								{Name: "PULL_PULL_SHA", Value: "pull-sha"},
								{Name: "PULL_REFS", Value: "base-ref:base-sha,1:pull-sha"},
								{Name: "REPO_NAME", Value: "repo-name"},
								{Name: "REPO_OWNER", Value: "org-name"},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "tools",
									MountPath: "/tools",
								},
							},
						},
						{
							Name:    "sidecar",
							Image:   "sidecar:tag",
							Command: []string{"/sidecar"},
							Env: []v1.EnvVar{
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}],"path_alias":"somewhere/else"}}`},
								{Name: "SIDECAR_OPTIONS", Value: `{"gcs_options":{"items":["/logs/artifacts"],"bucket":"my-bucket","path_strategy":"legacy","default_org":"kubernetes","default_repo":"kubernetes","gcs_credentials_file":"/secrets/gcs/service-account.json","dry_run":false},"wrapper_options":{"process_log":"/logs/process-log.txt","marker_file":"/logs/marker-file.txt"}}`},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "gcs-credentials",
									MountPath: "/secrets/gcs",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "logs",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "tools",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "gcs-credentials",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "secret-name",
								},
							},
						},
					},
				},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pj := kube.ProwJob{ObjectMeta: metav1.ObjectMeta{Name: test.podName, Labels: test.labels}, Spec: test.pjSpec}
		got, err := ProwJobToPod(pj, test.buildID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !equality.Semantic.DeepEqual(got, test.expected) {
			t.Errorf("expected pod diff:\n%s", diff.ObjectReflectDiff(test.expected, got))
		}
	}
}
