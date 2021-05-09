package main

import (
	"k8s.io/test-infra/prow/flagutil"
	"reflect"
	"testing"
)

func Test_options_getLabels(t *testing.T) {
	type fields struct {
		github     flagutil.GitHubOptions
		branch     string
		allowMods  bool
		confirm    bool
		local      bool
		org        string
		repo       string
		source     string
		title      string
		headBranch string
		matchTitle string
		body       string
		labels     string
	}
	tests := []struct {
		name   string
		fields fields
		want   []string
	}{
		{
			name: "empty labels",
			fields: fields{
				github:     flagutil.GitHubOptions{},
				branch:     "",
				allowMods:  false,
				confirm:    false,
				local:      false,
				org:        "",
				repo:       "",
				source:     "",
				title:      "",
				headBranch: "",
				matchTitle: "",
				body:       "",
				labels:     "",
			},
			want: []string{},
		},
		{
			name: "one label",
			fields: fields{
				github:     flagutil.GitHubOptions{},
				branch:     "",
				allowMods:  false,
				confirm:    false,
				local:      false,
				org:        "",
				repo:       "",
				source:     "",
				title:      "",
				headBranch: "",
				matchTitle: "",
				body:       "",
				labels:     "lgtm",
			},
			want: []string{
				"lgtm",
			},
		},
		{
			name: "two labels",
			fields: fields{
				github:     flagutil.GitHubOptions{},
				branch:     "",
				allowMods:  false,
				confirm:    false,
				local:      false,
				org:        "",
				repo:       "",
				source:     "",
				title:      "",
				headBranch: "",
				matchTitle: "",
				body:       "",
				labels:     "lgtm,approve",
			},
			want: []string{
				"lgtm",
				"approve",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := options{
				github:     tt.fields.github,
				branch:     tt.fields.branch,
				allowMods:  tt.fields.allowMods,
				confirm:    tt.fields.confirm,
				local:      tt.fields.local,
				org:        tt.fields.org,
				repo:       tt.fields.repo,
				source:     tt.fields.source,
				title:      tt.fields.title,
				headBranch: tt.fields.headBranch,
				matchTitle: tt.fields.matchTitle,
				body:       tt.fields.body,
				labels:     tt.fields.labels,
			}
			if got := o.getLabels(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getLabels() = %v, want %v", got, tt.want)
			}
		})
	}

}
