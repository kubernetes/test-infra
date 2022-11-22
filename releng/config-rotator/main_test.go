/*
Copyright 2019 The Kubernetes Authors.

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

package main

import "testing"

func TestUpdateGenericVersionMarker(t *testing.T) {
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
			if got := updateGenericVersionMarker(tt.args.s); got != tt.want {
				t.Errorf("updateGenericVersionMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}
