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

import (
	"flag"
	"reflect"
	"testing"

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
		err      bool
	}{{
		name:     "defaults don't work (set --config to prow config.yaml file)",
		expected: &options{},
		err:      true,
	}, {
		name: "only config works",
		args: []string{"--config=/etc/config.yaml"},
		expected: &options{
			config: configflagutil.ConfigOptions{
				ConfigPathFlagName:                    "config",
				ConfigPath:                            "/etc/config.yaml",
				JobConfigPathFlagName:                 "job-config-path",
				SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
			},
			instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
		},
	}, {
		name: "parse all arguments",
		args: []string{"--all-contexts=true", "--tot-url=https://tot",
			"--kubeconfig=/root/kubeconfig", "--config=/etc/config.yaml"},
		expected: &options{
			allContexts: true,
			totURL:      "https://tot",
			kubeconfig:  "/root/kubeconfig",
			config: configflagutil.ConfigOptions{
				ConfigPathFlagName:                    "config",
				ConfigPath:                            "/etc/config.yaml",
				JobConfigPathFlagName:                 "job-config-path",
				SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
			},
			instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actual options
			switch err := actual.parse(flags, tc.args); {
			case tc.expected == nil:
				if err == nil {
					t.Error("failed to receive an error")
				}
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to received expected error")
			case !reflect.DeepEqual(&actual, tc.expected):
				t.Errorf("actual %#v != expected %#v", actual, *tc.expected)
			}
		})
	}
}
