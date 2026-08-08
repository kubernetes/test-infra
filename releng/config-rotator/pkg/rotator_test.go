/*
Copyright The Kubernetes Authors.

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

package rotator

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/prow/pkg/config"
)

func TestRotateSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "tier suffix rotates",
			in:   "ci-kubernetes-e2e-gce-cos-default-beta",
			want: "ci-kubernetes-e2e-gce-cos-default-stable1",
		},
		{
			name: "beta feature gates stay",
			in:   "pull-kubernetes-e2e-kind-alpha-beta-features",
			want: "pull-kubernetes-e2e-kind-alpha-beta-features",
		},
		{
			name: "beta without a dash stays",
			in:   "pull-kubernetes-betabeta",
			want: "pull-kubernetes-betabeta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := rotateSuffix(tt.in, "beta", "stable1"); got != tt.want {
				t.Errorf("rotateSuffix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateJobBaseArgs(t *testing.T) {
	t.Parallel()

	job := config.JobBase{
		Name: "ci-kubernetes-e2e-gce-cos-default-beta",
		Spec: &v1.PodSpec{
			Containers: []v1.Container{{
				Args: []string{
					"--extra-version-markers=k8s-beta",
					"--runtime-config=api/alpha=true,api/beta=true",
				},
			}},
		},
	}

	updateJobBase(&job, "beta", "stable1")

	if want := "ci-kubernetes-e2e-gce-cos-default-stable1"; job.Name != want {
		t.Errorf("Name = %v, want %v", job.Name, want)
	}

	want := []string{
		"--extra-version-markers=k8s-stable1",
		"--runtime-config=api/alpha=true,api/beta=true",
	}
	for idx, arg := range job.Spec.Containers[0].Args {
		if arg != want[idx] {
			t.Errorf("Args[%d] = %v, want %v", idx, arg, want[idx])
		}
	}
}

func TestUpdateGenericVersionMarker(t *testing.T) {
	t.Parallel()

	type args struct {
		s      string
		marker string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "k8s-master",
			args: args{
				s:      "--extra-version-markers=k8s-master",
				marker: markerDefault,
			},
			want: "--extra-version-markers=k8s-beta",
		},
		{
			name: "k8s-beta",
			args: args{
				s:      "--extra-version-markers=k8s-beta",
				marker: markerBeta,
			},
			want: "--extra-version-markers=k8s-stable1",
		},
		{
			name: "k8s-stable1",
			args: args{
				s:      "--extra-version-markers=k8s-stable1",
				marker: markerStableOne,
			},
			want: "--extra-version-markers=k8s-stable2",
		},
		{
			name: "k8s-stable2",
			args: args{
				s:      "--extra-version-markers=k8s-stable2",
				marker: markerStableTwo,
			},
			want: "--extra-version-markers=k8s-stable3",
		},
		{
			name: "k8s-stable3",
			args: args{
				s:      "--extra-version-markers=k8s-stable3",
				marker: markerStableTwo,
			},
			want: "--extra-version-markers=k8s-stable4",
		},
		{
			name: "noReplace",
			args: args{
				s:      "no-replace",
				marker: "no",
			},
			want: "no-replace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := updateGenericVersionMarker(tt.args.s); got != tt.want {
				t.Errorf("updateGenericVersionMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}
