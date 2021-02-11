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

package main

import (
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/config/yamlcfg"
	"github.com/GoogleCloudPlatform/testgrid/pb/config"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowConfig "k8s.io/test-infra/prow/config"
)

const ProwDefaultGCSPath = "pathPrefix/"
const ProwJobName = "TestJob"
const ProwJobSourcePath = "jobs/org/repo/testjob.yaml"
const ProwJobURLPrefix = "https://go.k8s.io/prowjobs/"
const ExampleRepository = "test/repo"
const ProwJobDefaultDescription = "prowjob_name: " + ProwJobName

func Test_applySingleProwjobAnnotations(t *testing.T) {
	tests := []*struct {
		name              string
		initialConfig     config.Configuration
		updateDescription bool
		prowJobURLPrefix  string
		prowJobType       prowapi.ProwJobType
		annotations       map[string]string
		expectedConfig    config.Configuration
		expectError       bool
	}{
		{
			name:           "Presubmit with no Annotations: no change",
			prowJobType:    prowapi.PresubmitJob,
			expectedConfig: config.Configuration{},
		},
		{
			name:        "Non-presubmit with no Annotations: test group only",
			prowJobType: prowapi.PostsubmitJob,
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: ProwDefaultGCSPath + "logs/" + ProwJobName,
					},
				},
			},
		},
		{
			name:        "Presubmit forcing test group creation: sets hardcoded default",
			prowJobType: prowapi.PresubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group": "true",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:             ProwJobName,
						GcsPrefix:        ProwDefaultGCSPath + "pr-logs/directory/" + ProwJobName,
						NumColumnsRecent: 20,
					},
				},
			},
		},
		{
			name:        "Set columns below hardcoded minimum: annotation overwrites",
			prowJobType: prowapi.PresubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group":  "true",
				"testgrid-num-columns-recent": "10",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:             ProwJobName,
						GcsPrefix:        ProwDefaultGCSPath + "pr-logs/directory/" + ProwJobName,
						NumColumnsRecent: 10,
					},
				},
			},
		},
		{
			name:        "Non-presubmit excluding test group",
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group": "false",
			},
			expectedConfig: config.Configuration{},
		},
		{
			name: "Force-add job to existing test group: fails",
			initialConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{Name: ProwJobName},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group": "true",
			},
			expectError: true,
		},
		{
			name: "Add job to existing dashboard",
			initialConfig: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Wash"},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards": "Wash",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: ProwDefaultGCSPath + "logs/" + ProwJobName,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Wash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
							},
						},
					},
				},
			},
		},
		{
			name:        "Add job to new dashboard: fails",
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards": "Black",
			},
			expectError: true,
		},
		{
			name: "Add email to multiple dashboards: Two tabs, one email",
			initialConfig: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Dart"},
					{Name: "Peg"},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards":  "Dart, Peg",
				"testgrid-alert-email": "test@example.com",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: ProwDefaultGCSPath + "logs/" + ProwJobName,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Dart",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "test@example.com",
								},
							},
						},
					},
					{
						Name: "Peg",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Add job that already exists: keeps test group, makes duplicate tab",
			initialConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: "CustomFoo",
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Surf",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   "RegularHumanDescription",
								TestGroupName: ProwJobName,
							},
						},
					},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards": "Surf",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: "CustomFoo",
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Surf",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   "RegularHumanDescription",
								TestGroupName: ProwJobName,
							},
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Add job to existing dashboard with --prowjob-url-prefix configured",
			initialConfig: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Wash"},
				},
			},
			prowJobURLPrefix: ProwJobURLPrefix,
			prowJobType:      prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards": "Wash",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:      ProwJobName,
						GcsPrefix: ProwDefaultGCSPath + "logs/" + ProwJobName,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Wash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription + "\nprowjob_config_url: " + ProwJobURLPrefix + ProwJobSourcePath,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Full Annotations with --update-description enabled",
			initialConfig: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Ouija"},
				},
			},
			updateDescription: true,
			prowJobType:       prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards":                "Ouija",
				"testgrid-tab-name":                  "Planchette",
				"testgrid-alert-email":               "ghost@example.com",
				"description":                        "spooky scary",
				"testgrid-num-columns-recent":        "13",
				"testgrid-num-failures-to-alert":     "4",
				"testgrid-alert-stale-results-hours": "24",
				"testgrid-days-of-results":           "30",
				"testgrid-in-cell-metric":            "haunted-house",
			},
			expectedConfig: config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                   ProwJobName,
						GcsPrefix:              ProwDefaultGCSPath + "logs/" + ProwJobName,
						NumColumnsRecent:       13,
						NumFailuresToAlert:     4,
						AlertStaleResultsHours: 24,
						DaysOfResults:          30,
						ShortTextMetric:        "haunted-house",
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Ouija",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "Planchette",
								Description:   ProwJobDefaultDescription + "\nprowjob_description: spooky scary",
								TestGroupName: ProwJobName,
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "ghost@example.com",
								},
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pac := prowAwareConfigurator{
				prowConfig:            fakeProwConfig(),
				defaultTestgridConfig: nil,
				prowJobURLPrefix:      test.prowJobURLPrefix,
				updateDescription:     test.updateDescription,
			}
			job := prowConfig.JobBase{
				Name:        ProwJobName,
				Annotations: test.annotations,
				SourcePath:  ProwJobSourcePath,
			}

			err := pac.applySingleProwjobAnnotations(&test.initialConfig, job, test.prowJobType, ExampleRepository)

			if test.expectError {
				if err == nil {
					t.Error("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if !reflect.DeepEqual(&test.initialConfig, &test.expectedConfig) {
					t.Errorf("Configurations did not match; got %s, expected %s", test.initialConfig.String(), test.expectedConfig.String())
				}
			}
		})
	}
}

func Test_applySingleProwjobAnnotation_WithDefaults(t *testing.T) {

	defaultConfig := &yamlcfg.DefaultConfiguration{
		DefaultTestGroup: &config.TestGroup{
			GcsPrefix:        "originalConfigPrefix", //Default is Overwritten
			DaysOfResults:    5,                      //Default is Kept
			NumColumnsRecent: 10,                     //Sometimes Overwritten; see test
		},
		DefaultDashboardTab: &config.DashboardTab{
			Name:        "DefaultTab",          //Overwritten
			Description: "Default Description", //Overwritten
			ResultsText: "Default Text",        //Kept
			AlertOptions: &config.DashboardTabAlertOptions{
				AlertMailToAddresses: "default_admin@example.com", //Kept; see test
			},
			CodeSearchUrlTemplate: &config.LinkTemplate{ //Overwritten
				Url: "https://example.com/code_search",
			},
			OpenBugTemplate: &config.LinkTemplate{ //Overwritten
				Url: "https://example.com/open_bug",
			},
		},
	}

	tests := []struct {
		name           string
		initialConfig  *config.Configuration
		prowJobType    prowapi.ProwJobType
		annotations    map[string]string
		expectedConfig *config.Configuration
	}{
		{
			name:           "Presubmit with no Annotations: no change",
			prowJobType:    prowapi.PresubmitJob,
			expectedConfig: &config.Configuration{},
		},
		{
			name:        "Non-presubmit with no Annotations: test group with assumed defaults",
			prowJobType: prowapi.PostsubmitJob,
			expectedConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                ProwJobName,
						GcsPrefix:           ProwDefaultGCSPath + "logs/" + ProwJobName,
						DaysOfResults:       5,
						NumColumnsRecent:    10,
						UseKubernetesClient: true,
						IsExternal:          true,
					},
				},
			},
		},
		{
			name:        "Presubmit forcing test group creation: hardcoded default over config default",
			prowJobType: prowapi.PresubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group": "true",
			},
			expectedConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                ProwJobName,
						GcsPrefix:           ProwDefaultGCSPath + "pr-logs/directory/" + ProwJobName,
						DaysOfResults:       5,
						NumColumnsRecent:    20,
						UseKubernetesClient: true,
						IsExternal:          true,
					},
				},
			},
		},
		{
			name: "Add job to existing dashboard: merge with defaults",
			initialConfig: &config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Wash"},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards": "Wash",
			},
			expectedConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                ProwJobName,
						GcsPrefix:           ProwDefaultGCSPath + "logs/" + ProwJobName,
						DaysOfResults:       5,
						NumColumnsRecent:    10,
						UseKubernetesClient: true,
						IsExternal:          true,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Wash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "default_admin@example.com",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Add email to multiple dashboards: Two tabs, Different Emails",
			initialConfig: &config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Dart"},
					{Name: "Peg"},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards":  "Dart, Peg",
				"testgrid-alert-email": "test@example.com",
			},
			expectedConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                ProwJobName,
						GcsPrefix:           ProwDefaultGCSPath + "logs/" + ProwJobName,
						DaysOfResults:       5,
						NumColumnsRecent:    10,
						UseKubernetesClient: true,
						IsExternal:          true,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Dart",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "test@example.com",
								},
							},
						},
					},
					{
						Name: "Peg",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          ProwJobName,
								Description:   ProwJobDefaultDescription,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/issues/",
								},
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "default_admin@example.com",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			if test.initialConfig == nil {
				test.initialConfig = &config.Configuration{}
			}

			pac := prowAwareConfigurator{
				prowConfig:            fakeProwConfig(),
				defaultTestgridConfig: defaultConfig,
			}

			job := prowConfig.JobBase{
				Name:        ProwJobName,
				Annotations: test.annotations,
			}

			err := pac.applySingleProwjobAnnotations(test.initialConfig, job, test.prowJobType, ExampleRepository)

			if test.expectedConfig == nil {
				if err == nil {
					t.Error("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if !reflect.DeepEqual(test.initialConfig, test.expectedConfig) {
					t.Errorf("Configurations did not match; got %s, expected %s", test.initialConfig.String(), test.expectedConfig.String())
				}
			}
		})
	}

}

func TestSortPresubmitRepoOrder(t *testing.T) {
	tests := []struct {
		name          string
		presubmits    map[string][]prowConfig.Presubmit
		expectedRepos []string
	}{
		{
			name:          "empty list of presubmits",
			presubmits:    map[string][]prowConfig.Presubmit{},
			expectedRepos: []string{},
		},
		{
			name: "unordered list of presubmits",
			presubmits: map[string][]prowConfig.Presubmit{
				"istio/proxy": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_release-1.5",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "gen_check_master",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_master",
						},
					},
				},
				"kubernetes/test-infra": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-bazel",
						},
					},
				},
				"helm/helm": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-go",
						},
					},
				},
			},
			expectedRepos: []string{"helm/helm", "istio/proxy", "kubernetes/test-infra"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualRepos := sortPresubmits(test.presubmits)

			if !reflect.DeepEqual(test.expectedRepos, actualRepos) {
				t.Fatalf("Presubmit repos do not match; actual: %v\n expected %v\n", test.expectedRepos, actualRepos)
			}
		})
	}
}

func TestSortPostsubmitRepoOrder(t *testing.T) {
	tests := []struct {
		name          string
		postsubmits   map[string][]prowConfig.Postsubmit
		expectedRepos []string
	}{
		{
			name:          "empty list of postsubmits",
			postsubmits:   map[string][]prowConfig.Postsubmit{},
			expectedRepos: []string{},
		},
		{
			name: "unordered list of postsubmits",
			postsubmits: map[string][]prowConfig.Postsubmit{
				"GoogleCloudPlatform/oss-test-infra": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-infra-go-test",
						},
					},
				},
				"kubernetes/kubernetes": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-e2e",
						},
					},
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-unit",
						},
					},
				},
				"containerd/cri": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-cri-containerd-build",
						},
					},
				},
			},
			expectedRepos: []string{"GoogleCloudPlatform/oss-test-infra", "containerd/cri", "kubernetes/kubernetes"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualRepos := sortPostsubmits(test.postsubmits)

			if !reflect.DeepEqual(test.expectedRepos, actualRepos) {
				t.Fatalf("Postsubmit repos do not match; actual: %v\n expected %v\n", test.expectedRepos, actualRepos)
			}
		})
	}
}

func TestSortPeriodicJobOrder(t *testing.T) {
	tests := []struct {
		name              string
		periodics         []prowConfig.Periodic
		expectedPeriodics []prowConfig.Periodic
	}{
		{
			name:              "empty list of periodics",
			periodics:         []prowConfig.Periodic{},
			expectedPeriodics: []prowConfig.Periodic{},
		},
		{
			name: "unordered list of periodics",
			periodics: []prowConfig.Periodic{
				{
					JobBase: prowConfig.JobBase{
						Name: "ESPv2-continuous-build",
					},
				},
				{
					JobBase: prowConfig.JobBase{
						Name: "everlast-bump",
					},
				},
				{
					JobBase: prowConfig.JobBase{
						Name: "ci-oss-test-infra-autobump-prow",
					},
				},
			},
			expectedPeriodics: []prowConfig.Periodic{
				{
					JobBase: prowConfig.JobBase{
						Name: "ESPv2-continuous-build",
					},
				},
				{
					JobBase: prowConfig.JobBase{
						Name: "ci-oss-test-infra-autobump-prow",
					},
				},
				{
					JobBase: prowConfig.JobBase{
						Name: "everlast-bump",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sortPeriodics(test.periodics)

			if !reflect.DeepEqual(test.expectedPeriodics, test.periodics) {
				t.Fatalf("Periodic jobs do not match; actual: %v\n expected %v\n", test.expectedPeriodics, test.periodics)
			}
		})
	}
}

func TestSortPresubmitJobOrder(t *testing.T) {
	tests := []struct {
		name               string
		presubmits         map[string][]prowConfig.Presubmit
		expectedPresubmits map[string][]prowConfig.Presubmit
	}{
		{
			name:               "empty list of presubmits",
			presubmits:         map[string][]prowConfig.Presubmit{},
			expectedPresubmits: map[string][]prowConfig.Presubmit{},
		},
		{
			name: "unordered list of presubmits",
			presubmits: map[string][]prowConfig.Presubmit{
				"istio/proxy": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_release-1.5",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "gen_check_master",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_master",
						},
					},
				},
				"kubernetes/test-infra": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-go",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-bazel",
						},
					},
				},
			},
			expectedPresubmits: map[string][]prowConfig.Presubmit{
				"istio/proxy": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "gen_check_master",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_master",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "lint_release-1.5",
						},
					},
				},
				"kubernetes/test-infra": {
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-bazel",
						},
					},
					prowConfig.Presubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-go",
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sortPresubmits(test.presubmits)

			for orgrepo := range test.expectedPresubmits {
				if !reflect.DeepEqual(test.expectedPresubmits[orgrepo], test.presubmits[orgrepo]) {
					t.Fatalf("Presubmit jobs do not match for repo: %s; actual: %v\n expected %v\n", orgrepo, test.expectedPresubmits[orgrepo], test.presubmits[orgrepo])
				}
			}
		})
	}
}

func TestSortPostsubmitJobOrder(t *testing.T) {
	tests := []struct {
		name                string
		postsubmits         map[string][]prowConfig.Postsubmit
		expectedPostsubmits map[string][]prowConfig.Postsubmit
	}{
		{
			name:                "empty list of postsubmits",
			postsubmits:         map[string][]prowConfig.Postsubmit{},
			expectedPostsubmits: map[string][]prowConfig.Postsubmit{},
		},
		{
			name: "unordered list of postsubmits",
			postsubmits: map[string][]prowConfig.Postsubmit{
				"GoogleCloudPlatform/oss-test-infra": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-infra-go-test",
						},
					},
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-cri-containerd-build",
						},
					},
				},
				"kubernetes/kubernetes": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-e2e",
						},
					},
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-unit",
						},
					},
				},
			},
			expectedPostsubmits: map[string][]prowConfig.Postsubmit{
				"GoogleCloudPlatform/oss-test-infra": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-cri-containerd-build",
						},
					},
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "pull-test-infra-go-test",
						},
					},
				},
				"kubernetes/kubernetes": {
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-e2e",
						},
					},
					prowConfig.Postsubmit{
						JobBase: prowConfig.JobBase{
							Name: "ci-kubernetes-unit",
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sortPostsubmits(test.postsubmits)

			for orgrepo := range test.expectedPostsubmits {
				if !reflect.DeepEqual(test.expectedPostsubmits[orgrepo], test.postsubmits[orgrepo]) {
					t.Fatalf("Postsubmit jobs do not match for repo: %s; actual: %v\n expected %v\n", orgrepo, test.expectedPostsubmits[orgrepo], test.postsubmits[orgrepo])
				}
			}
		})
	}
}

func fakeProwConfig() *prowConfig.Config {
	return &prowConfig.Config{
		ProwConfig: prowConfig.ProwConfig{
			Plank: prowConfig.Plank{
				DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
					"*": {
						GCSConfiguration: &prowapi.GCSConfiguration{
							PathPrefix: ProwDefaultGCSPath,
						},
					},
				},
			},
		},
	}
}
