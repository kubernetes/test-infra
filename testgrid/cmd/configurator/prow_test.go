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
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowConfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/testgrid/config"
	"reflect"
	"testing"
)

const ProwDefaultGCSPath = "pathPrefix/"
const ProwJobName = "TestJob"

func Test_applySingleProwjobAnnotations(t *testing.T) {
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
			name:        "Non-presubmit with no Annotations: test group only",
			prowJobType: prowapi.PostsubmitJob,
			expectedConfig: &config.Configuration{
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
			expectedConfig: &config.Configuration{
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
			expectedConfig: &config.Configuration{
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
			expectedConfig: &config.Configuration{},
		},
		{
			name: "Force-add job to existing test group: fails",
			initialConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{Name: ProwJobName},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-create-test-group": "true",
			},
		},
		{
			name: "Add job to existing dashboard",
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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
		},
		{
			name: "Add email to multiple dashboards: Two tabs, one email",
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Add job that already exists: keeps test group, makes duplicate tab",
			initialConfig: &config.Configuration{
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
			expectedConfig: &config.Configuration{
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Full Annotations",
			initialConfig: &config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Ouija"},
				},
			},
			prowJobType: prowapi.PostsubmitJob,
			annotations: map[string]string{
				"testgrid-dashboards":                "Ouija",
				"testgrid-tab-name":                  "Planchette",
				"testgrid-alert-email":               "ghost@example.com",
				"description":                        "spooky scary",
				"testgrid-num-columns-recent":        "13",
				"testgrid-num-failures-to-alert":     "4",
				"testgrid-alert-stale-results-hours": "24",
			},
			expectedConfig: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                   ProwJobName,
						GcsPrefix:              ProwDefaultGCSPath + "logs/" + ProwJobName,
						NumColumnsRecent:       13,
						NumFailuresToAlert:     4,
						AlertStaleResultsHours: 24,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "Ouija",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "Planchette",
								Description:   "spooky scary",
								TestGroupName: ProwJobName,
								AlertOptions: &config.DashboardTabAlertOptions{
									AlertMailToAddresses: "ghost@example.com",
								},
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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

			result := &Config{
				config: test.initialConfig,
			}

			job := prowConfig.JobBase{
				Name:        ProwJobName,
				Annotations: test.annotations,
			}

			err := applySingleProwjobAnnotations(result, fakeProwConfig(), job, test.prowJobType, "test/repo")

			if test.expectedConfig == nil {
				if err == nil {
					t.Error("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if !reflect.DeepEqual(result.config, test.expectedConfig) {
					t.Errorf("Configurations did not match; got %s, expected %s", result.config.String(), result.config.String())
				}
			}
		})
	}
}

func Test_applySingleProwjobAnnotation_WithDefaults(t *testing.T) {

	defaultConfig := &config.DefaultConfiguration{
		DefaultTestGroup: &config.TestGroup{
			GcsPrefix:        "originalConfigPrefix", //Overwritten
			DaysOfResults:    5,                      //Kept
			NumColumnsRecent: 10,                     //Sometimes Overwritten; see test
		},
		DefaultDashboardTab: &config.DashboardTab{
			Name:        "DefaultTab",          //Overwritten
			Description: "Default Description", //Overwritten
			ResultsText: "Default Text",        //Kept
			AlertOptions: &config.DashboardTabAlertOptions{
				AlertMailToAddresses: "default_admin@example.com", //Kept; see test
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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
								Description:   ProwJobName,
								TestGroupName: ProwJobName,
								ResultsText:   "Default Text",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "https://github.com/test/repo/compare/<start-custom-0>...<end-custom-0>",
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

			result := &Config{
				config:        test.initialConfig,
				defaultConfig: defaultConfig,
			}

			job := prowConfig.JobBase{
				Name:        ProwJobName,
				Annotations: test.annotations,
			}

			err := applySingleProwjobAnnotations(result, fakeProwConfig(), job, test.prowJobType, "test/repo")

			if test.expectedConfig == nil {
				if err == nil {
					t.Error("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if !reflect.DeepEqual(result.defaultConfig, defaultConfig) {
					t.Errorf("Default Configuration should not change; got %s, expected %s,", result.defaultConfig.String(), defaultConfig.String())
				}

				if !reflect.DeepEqual(result.config, test.expectedConfig) {
					t.Errorf("Configurations did not match; got %s, expected %s", result.config.String(), test.expectedConfig.String())
				}
			}
		})
	}

}

func fakeProwConfig() *prowConfig.Config {
	return &prowConfig.Config{
		ProwConfig: prowConfig.ProwConfig{
			Plank: prowConfig.Plank{
				DefaultDecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathPrefix: ProwDefaultGCSPath,
					},
				},
			},
		},
	}
}
