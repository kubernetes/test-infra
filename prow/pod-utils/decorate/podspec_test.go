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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/clonerefs"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/initupload"
	"k8s.io/test-infra/prow/sidecar"
)

func pStr(str string) *string {
	return &str
}

func cookieVolumeOnly(secret string) coreapi.Volume {
	v, _, _ := cookiefileVolume(secret)
	return v
}

func cookieMountOnly(secret string) coreapi.VolumeMount {
	_, vm, _ := cookiefileVolume(secret)
	return vm
}
func cookiePathOnly(secret string) string {
	_, _, vp := cookiefileVolume(secret)
	return vp
}

func TestCloneRefs(t *testing.T) {
	truth := true
	logMount := coreapi.VolumeMount{
		Name:      "log",
		MountPath: "/log-mount",
	}
	codeMount := coreapi.VolumeMount{
		Name:      "code",
		MountPath: "/code-mount",
	}
	tmpMount := coreapi.VolumeMount{
		Name:      "clonerefs-tmp",
		MountPath: "/tmp",
	}
	tmpVolume := coreapi.Volume{
		Name: "clonerefs-tmp",
		VolumeSource: coreapi.VolumeSource{
			EmptyDir: &coreapi.EmptyDirVolumeSource{},
		},
	}
	envOrDie := func(opt clonerefs.Options) []coreapi.EnvVar {
		e, err := cloneEnv(opt)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	sshVolumeOnly := func(secret string) coreapi.Volume {
		v, _ := sshVolume(secret)
		return v
	}

	sshMountOnly := func(secret string) coreapi.VolumeMount {
		_, vm := sshVolume(secret)
		return vm
	}

	cases := []struct {
		name              string
		pj                prowapi.ProwJob
		codeMountOverride *coreapi.VolumeMount
		logMountOverride  *coreapi.VolumeMount
		expected          *coreapi.Container
		volumes           []coreapi.Volume
		err               bool
	}{
		{
			name: "empty returns nil",
		},
		{
			name: "nil refs and extrarefs returns nil",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					DecorationConfig: &prowapi.DecorationConfig{},
				},
			},
		},
		{
			name: "nil DecorationConfig returns nil",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{},
				},
			},
		},
		{
			name: "SkipCloning returns nil",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{},
					DecorationConfig: &prowapi.DecorationConfig{
						SkipCloning: &truth,
					},
				},
			},
		},
		{
			name: "reject empty code mount name",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					DecorationConfig: &prowapi.DecorationConfig{},
					Refs:             &prowapi.Refs{},
				},
			},
			codeMountOverride: &coreapi.VolumeMount{
				MountPath: "/whatever",
			},
			err: true,
		},
		{
			name: "reject empty code mountpath",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					DecorationConfig: &prowapi.DecorationConfig{},
					Refs:             &prowapi.Refs{},
				},
			},
			codeMountOverride: &coreapi.VolumeMount{
				Name: "wee",
			},
			err: true,
		},
		{
			name: "reject empty log mount name",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					DecorationConfig: &prowapi.DecorationConfig{},
					Refs:             &prowapi.Refs{},
				},
			},
			logMountOverride: &coreapi.VolumeMount{
				MountPath: "/whatever",
			},
			err: true,
		},
		{
			name: "reject empty log mountpath",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					DecorationConfig: &prowapi.DecorationConfig{},
					Refs:             &prowapi.Refs{},
				},
			},
			logMountOverride: &coreapi.VolumeMount{
				Name: "wee",
			},
			err: true,
		},
		{
			name: "create clonerefs container when refs are set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:      []prowapi.Refs{{}},
					GitUserEmail: clonerefs.DefaultGitUserEmail,
					GitUserName:  clonerefs.DefaultGitUserName,
					SrcRoot:      codeMount.MountPath,
					Log:          CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount, tmpMount},
			},
			volumes: []coreapi.Volume{tmpVolume},
		},
		{
			name: "create clonerefs containers when extrarefs are set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					ExtraRefs: []prowapi.Refs{{}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:      []prowapi.Refs{{}},
					GitUserEmail: clonerefs.DefaultGitUserEmail,
					GitUserName:  clonerefs.DefaultGitUserName,
					SrcRoot:      codeMount.MountPath,
					Log:          CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount, tmpMount},
			},
			volumes: []coreapi.Volume{tmpVolume},
		},
		{
			name: "append extrarefs after refs",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs:      &prowapi.Refs{Org: "first"},
					ExtraRefs: []prowapi.Refs{{Org: "second"}, {Org: "third"}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:      []prowapi.Refs{{Org: "first"}, {Org: "second"}, {Org: "third"}},
					GitUserEmail: clonerefs.DefaultGitUserEmail,
					GitUserName:  clonerefs.DefaultGitUserName,
					SrcRoot:      codeMount.MountPath,
					Log:          CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount, tmpMount},
			},
			volumes: []coreapi.Volume{tmpVolume},
		},
		{
			name: "append ssh secrets when set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
						SSHKeySecrets: []string{"super", "secret"},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:      []prowapi.Refs{{}},
					GitUserEmail: clonerefs.DefaultGitUserEmail,
					GitUserName:  clonerefs.DefaultGitUserName,
					KeyFiles:     []string{sshMountOnly("super").MountPath, sshMountOnly("secret").MountPath},
					SrcRoot:      codeMount.MountPath,
					Log:          CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{
					logMount,
					codeMount,
					sshMountOnly("super"),
					sshMountOnly("secret"),
					tmpMount,
				},
			},
			volumes: []coreapi.Volume{sshVolumeOnly("super"), sshVolumeOnly("secret"), tmpVolume},
		},
		{
			name: "include ssh host fingerprints when set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					ExtraRefs: []prowapi.Refs{{}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages:       &prowapi.UtilityImages{},
						SSHHostFingerprints: []string{"thumb", "pinky"},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:          []prowapi.Refs{{}},
					GitUserEmail:     clonerefs.DefaultGitUserEmail,
					GitUserName:      clonerefs.DefaultGitUserName,
					SrcRoot:          codeMount.MountPath,
					HostFingerprints: []string{"thumb", "pinky"},
					Log:              CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount, tmpMount},
			},
			volumes: []coreapi.Volume{tmpVolume},
		},
		{
			name: "include cookiefile secrets when set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					ExtraRefs: []prowapi.Refs{{}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages:    &prowapi.UtilityImages{},
						CookiefileSecret: "oatmeal",
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Args:    []string{"--cookiefile=" + cookiePathOnly("oatmeal")},
				Env: envOrDie(clonerefs.Options{
					CookiePath:   cookiePathOnly("oatmeal"),
					GitRefs:      []prowapi.Refs{{}},
					GitUserEmail: clonerefs.DefaultGitUserEmail,
					GitUserName:  clonerefs.DefaultGitUserName,
					SrcRoot:      codeMount.MountPath,
					Log:          CloneLogPath(logMount),
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount, tmpMount, cookieMountOnly("oatmeal")},
			},
			volumes: []coreapi.Volume{tmpVolume, cookieVolumeOnly("oatmeal")},
		},
		{
			name: "include oauth token secret when set",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					ExtraRefs: []prowapi.Refs{{}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
						OauthTokenSecret: &prowapi.OauthTokenSecret{
							Name: "oauth-secret",
							Key:  "oauth-file",
						},
					},
				},
			},
			expected: &coreapi.Container{
				Name:    cloneRefsName,
				Command: []string{cloneRefsCommand},
				Env: envOrDie(clonerefs.Options{
					GitRefs:        []prowapi.Refs{{}},
					GitUserEmail:   clonerefs.DefaultGitUserEmail,
					GitUserName:    clonerefs.DefaultGitUserName,
					SrcRoot:        codeMount.MountPath,
					Log:            CloneLogPath(logMount),
					OauthTokenFile: "/secrets/oauth/oauth-token",
				}),
				VolumeMounts: []coreapi.VolumeMount{logMount, codeMount,
					{Name: "oauth-secret", ReadOnly: true, MountPath: "/secrets/oauth"}, tmpMount,
				},
			},
			volumes: []coreapi.Volume{
				{
					Name: "oauth-secret",
					VolumeSource: coreapi.VolumeSource{
						Secret: &coreapi.SecretVolumeSource{
							SecretName: "oauth-secret",
							Items: []coreapi.KeyToPath{{
								Key:  "oauth-file",
								Path: "./oauth-token"}},
						},
					},
				},
				tmpVolume,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lm := logMount
			if tc.logMountOverride != nil {
				lm = *tc.logMountOverride
			}
			cm := codeMount
			if tc.codeMountOverride != nil {
				cm = *tc.codeMountOverride
			}
			actual, refs, volumes, err := CloneRefs(tc.pj, cm, lm)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive expected exception")
			case !equality.Semantic.DeepEqual(tc.expected, actual):
				t.Errorf("unexpected container:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			case !equality.Semantic.DeepEqual(tc.volumes, volumes):
				t.Errorf("unexpected volume:\n%s", diff.ObjectReflectDiff(tc.volumes, volumes))
			case actual != nil:
				var er []prowapi.Refs
				if tc.pj.Spec.Refs != nil {
					er = append(er, *tc.pj.Spec.Refs)
				}
				for _, r := range tc.pj.Spec.ExtraRefs {
					er = append(er, r)
				}
				if !equality.Semantic.DeepEqual(refs, er) {
					t.Errorf("unexpected refs:\n%s", diff.ObjectReflectDiff(er, refs))
				}
			}
		})
	}
}

func TestProwJobToPod(t *testing.T) {
	truth := true
	tests := []struct {
		podName  string
		buildID  string
		labels   map[string]string
		pjSpec   prowapi.ProwJobSpec
		pjStatus prowapi.ProwJobStatus
	}{
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: prowapi.ProwJobSpec{
				Type:  prowapi.PresubmitJob,
				Job:   "job-name",
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
				},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image: "tester",
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},
			pjStatus: prowapi.ProwJobStatus{
				BuildID: "blabla",
			},
		},
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
						MediaTypes:   map[string]string{"log": "text/plain"},
					},
					GCSCredentialsSecret: pStr("secret-name"),
					CookiefileSecret:     "yummy/.gitcookies",
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					CookiefileSecret:     "yummy",
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					SSHKeySecrets:        []string{"ssh-1", "ssh-2"},
					SSHHostFingerprints:  []string{"hello", "world"},
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
							TerminationMessagePolicy: coreapi.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					SSHKeySecrets:        []string{"ssh-1", "ssh-2"},
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PeriodicJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					SSHKeySecrets:        []string{"ssh-1", "ssh-2"},
				},
				Agent: prowapi.KubernetesAgent,
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					SSHKeySecrets:        []string{"ssh-1", "ssh-2"},
					SkipCloning:          &truth,
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{
					{
						Org:  "extra-org",
						Repo: "extra-repo",
					},
				},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
					GCSCredentialsSecret: pStr("secret-name"),
					SSHKeySecrets:        []string{"ssh-1", "ssh-2"},
					CookiefileSecret:     "yummy",
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{
					{
						Org:  "extra-org",
						Repo: "extra-repo",
					},
				},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Name:    "test-0",
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
						{
							Name:    "test-1",
							Image:   "othertester",
							Command: []string{"/bin/otherthing"},
							Args:    []string{"other", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "stones"},
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
			pjSpec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "job-name",
				DecorationConfig: &prowapi.DecorationConfig{
					Timeout:     &prowapi.Duration{Duration: 120 * time.Minute},
					GracePeriod: &prowapi.Duration{Duration: 10 * time.Second},
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:tag",
						InitUpload: "initupload:tag",
						Entrypoint: "entrypoint:tag",
						Sidecar:    "sidecar:tag",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "my-bucket",
						PathStrategy: "legacy",
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
						MediaTypes:   map[string]string{"log": "text/plain"},
					},
					// Specify K8s SA rather than cloud storage secret key.
					DefaultServiceAccountName: pStr("default-SA"),
					CookiefileSecret:          "yummy/.gitcookies",
				},
				Agent: prowapi.KubernetesAgent,
				Refs: &prowapi.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
					PathAlias: "somewhere/else",
				},
				ExtraRefs: []prowapi.Refs{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image:   "tester",
							Command: []string{"/bin/thing"},
							Args:    []string{"some", "args"},
							Env: []coreapi.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},
		},
	}

	findContainer := func(name string, pod coreapi.Pod) *coreapi.Container {
		for _, c := range pod.Spec.Containers {
			if c.Name == name {
				return &c
			}
		}
		return nil
	}
	findEnv := func(key string, container coreapi.Container) *string {
		for _, env := range container.Env {
			if env.Name == key {
				v := env.Value
				return &v
			}

		}
		return nil
	}

	type checker interface {
		ConfigVar() string
		LoadConfig(string) error
		Validate() error
	}

	checkEnv := func(pod coreapi.Pod, name string, opt checker) error {
		c := findContainer(name, pod)
		if c == nil {
			return nil
		}
		env := opt.ConfigVar()
		val := findEnv(env, *c)
		if val == nil {
			return fmt.Errorf("missing %s env var", env)
		}
		if err := opt.LoadConfig(*val); err != nil {
			return fmt.Errorf("load: %v", err)
		}
		if err := opt.Validate(); err != nil {
			return fmt.Errorf("validate: %v", err)
		}
		return nil
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			pj := prowapi.ProwJob{ObjectMeta: metav1.ObjectMeta{Name: test.podName, Labels: test.labels}, Spec: test.pjSpec, Status: test.pjStatus}
			pj.Status.BuildID = test.buildID
			got, err := ProwJobToPod(pj)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			fixtureName := filepath.Join("testdata", fmt.Sprintf("%s.yaml", strings.ReplaceAll(t.Name(), "/", "_")))
			if os.Getenv("UPDATE") != "" {
				marshalled, err := yaml.Marshal(got)
				if err != nil {
					t.Fatalf("failed to marhsal pod: %v", err)
				}
				if err := ioutil.WriteFile(fixtureName, marshalled, 0644); err != nil {
					t.Errorf("failed to update fixture: %v", err)
				}
			}
			expectedRaw, err := ioutil.ReadFile(fixtureName)
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}
			expected := &coreapi.Pod{}
			if err := yaml.Unmarshal(expectedRaw, expected); err != nil {
				t.Fatalf("failed to unmarshal fixture: %v", err)
			}
			if !equality.Semantic.DeepEqual(got, expected) {
				t.Errorf("unexpected pod diff:\n%s. You can update the fixtures by running this test with UPDATE=true if this is expected.", diff.ObjectReflectDiff(expected, got))
			}
			if err := checkEnv(*got, "sidecar", sidecar.NewOptions()); err != nil {
				t.Errorf("bad sidecar env: %v", err)
			}
			if err := checkEnv(*got, "initupload", initupload.NewOptions()); err != nil {
				t.Errorf("bad clonerefs env: %v", err)
			}
			if err := checkEnv(*got, "clonerefs", &clonerefs.Options{}); err != nil {
				t.Errorf("bad clonerefs env: %v", err)
			}
			if test.pjSpec.DecorationConfig != nil { // all jobs get a test container
				// But only decorated jobs need valid entrypoint options
				if err := checkEnv(*got, "test", entrypoint.NewOptions()); err != nil {
					t.Errorf("bad test entrypoint: %v", err)
				}
			}
		})
	}
}

func TestProwJobToPod_setsTerminationGracePeriodSeconds(t *testing.T) {
	testCases := []struct {
		name                                  string
		prowjob                               *prowapi.ProwJob
		expectedTerminationGracePeriodSeconds int64
	}{
		{
			name: "GracePeriodSeconds from decoration config",
			prowjob: &prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{}}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
						GracePeriod:   &prowapi.Duration{Duration: 10 * time.Second},
					},
				},
			},
			expectedTerminationGracePeriodSeconds: 10,
		},
		{
			name: "Existing GracePeriodSeconds is not overwritten",
			prowjob: &prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					PodSpec: &coreapi.PodSpec{TerminationGracePeriodSeconds: utilpointer.Int64Ptr(60), Containers: []coreapi.Container{{}}},
					DecorationConfig: &prowapi.DecorationConfig{
						UtilityImages: &prowapi.UtilityImages{},
						Timeout:       &prowapi.Duration{Duration: 10 * time.Second},
					},
				},
			},
			expectedTerminationGracePeriodSeconds: 60,
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := decorate(tc.prowjob.Spec.PodSpec, tc.prowjob, map[string]string{}, ""); err != nil {
				t.Fatalf("decoration failed: %v", err)
			}
			if tc.prowjob.Spec.PodSpec.TerminationGracePeriodSeconds == nil || *tc.prowjob.Spec.PodSpec.TerminationGracePeriodSeconds != tc.expectedTerminationGracePeriodSeconds {
				t.Errorf("expected pods TerminationGracePeriodSeconds to be %d was %v", tc.expectedTerminationGracePeriodSeconds, tc.prowjob.Spec.PodSpec.TerminationGracePeriodSeconds)
			}
		})
	}
}
