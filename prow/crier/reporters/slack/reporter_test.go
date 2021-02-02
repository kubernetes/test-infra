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

package slack

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func TestShouldReport(t *testing.T) {
	testCases := []struct {
		name     string
		config   config.SlackReporter
		pj       *v1.ProwJob
		expected bool
	}{
		{
			name: "Presubmit Job should report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{v1.PresubmitJob},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: true,
		},
		{
			name: "Presubmit Job should not report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{v1.PostsubmitJob},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: false,
		},
		{
			name: "Successful Job should report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{v1.PostsubmitJob},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: true,
		},
		{
			name: "Successful Job should not report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{v1.PostsubmitJob},
				JobStatesToReport: []v1.ProwJobState{v1.PendingState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: false,
		},
		{
			name: "Job with channel config should ignore the JobTypesToReport config",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						Slack: &v1.SlackReporterConfig{Channel: "whatever-channel"},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: true,
		},
		{
			name: "JobStatesToReport in Job config should override the one in Prow config",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						Slack: &v1.SlackReporterConfig{
							Channel:           "whatever-channel",
							JobStatesToReport: []v1.ProwJobState{v1.FailureState, v1.PendingState},
						},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
				},
			},
			expected: true,
		},
		{
			name: "Job with channel config but does not have matched state in Prow config should not report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						Slack: &v1.SlackReporterConfig{Channel: "whatever-channel"},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.PendingState,
				},
			},
			expected: false,
		},
		{
			name: "Job with channel and state config where the state does not match, should not report",
			config: config.SlackReporter{
				JobTypesToReport:  []v1.ProwJobType{},
				JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						Slack: &v1.SlackReporterConfig{
							Channel:           "whatever-channel",
							JobStatesToReport: []v1.ProwJobState{v1.FailureState, v1.PendingState},
						},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: false,
		},
		{
			name:   "Empty config should not report",
			config: config.SlackReporter{},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		cfgGetter := func(*v1.Refs) config.SlackReporter {
			return tc.config
		}
		t.Run(tc.name, func(t *testing.T) {
			reporter := &slackReporter{
				config: cfgGetter,
			}

			if result := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj); result != tc.expected {
				t.Errorf("expected result to be %t but was %t", tc.expected, result)
			}
		})
	}
}

func TestReloadsConfig(t *testing.T) {
	cfg := config.SlackReporter{}
	cfgGetter := func(*v1.Refs) config.SlackReporter {
		return cfg
	}

	pj := &v1.ProwJob{
		Spec: v1.ProwJobSpec{
			Type: v1.PostsubmitJob,
		},
		Status: v1.ProwJobStatus{
			State: v1.FailureState,
		},
	}

	reporter := &slackReporter{
		config: cfgGetter,
	}

	if shouldReport := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj); shouldReport {
		t.Error("Did expect shouldReport to be false")
	}

	cfg.JobStatesToReport = []v1.ProwJobState{v1.FailureState}
	cfg.JobTypesToReport = []v1.ProwJobType{v1.PostsubmitJob}

	if shouldReport := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj); !shouldReport {
		t.Error("Did expect shouldReport to be true after config change")
	}
}

func TestUsesChannelOverrideFromJob(t *testing.T) {
	testCases := []struct {
		name          string
		config        func() config.Config
		pj            *v1.ProwJob
		expected      string
		emptyExpected bool
	}{
		{
			name: "No job-level config, use global default",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"*": {
						Channel: "global-default",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj:       &v1.ProwJob{Spec: v1.ProwJobSpec{}},
			expected: "global-default",
		},
		{
			name: "org/repo for ref exists in config, use it",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"*": {
						Channel: "global-default",
					},
					"istio/proxy": {
						Channel: "org-repo-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Org:  "istio",
						Repo: "proxy",
					},
				}},
			expected: "org-repo-config",
		},
		{
			name: "org for ref exists in config, use it",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"*": {
						Channel: "global-default",
					},
					"istio": {
						Channel: "org-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Org:  "istio",
						Repo: "proxy",
					},
				}},
			expected: "org-config",
		},
		{
			name: "org/repo takes precedence over org",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"*": {
						Channel: "global-default",
					},
					"istio": {
						Channel: "org-config",
					},
					"istio/proxy": {
						Channel: "org-repo-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Org:  "istio",
						Repo: "proxy",
					},
				}},
			expected: "org-repo-config",
		},
		{
			name: "Job-level config present, use it",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"*": {
						Channel: "global-default",
					},
					"istio": {
						Channel: "org-config",
					},
					"istio/proxy": {
						Channel: "org-repo-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					ReporterConfig: &v1.ReporterConfig{
						Slack: &v1.SlackReporterConfig{
							Channel: "team-a",
						},
					},
				},
			},
			expected: "team-a",
		},
		{
			name: "No matching slack config",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"istio": {
						Channel: "org-config",
					},
					"istio/proxy": {
						Channel: "org-repo-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Org:  "unknownorg",
						Repo: "unknownrepo",
					},
				}},
			emptyExpected: true,
		},
		{
			name: "Refs unset but extra refs exist, use it",
			config: func() config.Config {
				slackCfg := map[string]config.SlackReporter{
					"istio/proxy": {
						Channel: "org-repo-config",
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					ExtraRefs: []v1.Refs{{
						Org:  "istio",
						Repo: "proxy",
					}},
				},
			},
			expected: "org-repo-config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfgGetter := func(refs *v1.Refs) config.SlackReporter {
				return tc.config().SlackReporterConfigs.GetSlackReporter(refs)
			}
			sr := slackReporter{
				config: cfgGetter,
			}

			prowSlackCfg := sr.getConfig(tc.pj)
			jobSlackCfg := jobConfig(tc.pj)
			if result := channel(prowSlackCfg, jobSlackCfg); result != tc.expected {
				t.Fatalf("Expected result to be %q, was %q", tc.expected, result)
			}
		})
	}
}

func TestShouldReportDefaultsToExtraRefs(t *testing.T) {
	job := &v1.ProwJob{
		Spec: v1.ProwJobSpec{
			Type:      v1.PeriodicJob,
			ExtraRefs: []v1.Refs{{Org: "org"}},
		},
		Status: v1.ProwJobStatus{
			State: v1.SuccessState,
		},
	}
	sr := slackReporter{
		config: func(r *v1.Refs) config.SlackReporter {
			if r.Org == "org" {
				return config.SlackReporter{
					JobTypesToReport:  []v1.ProwJobType{v1.PeriodicJob},
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				}
			}
			return config.SlackReporter{}
		},
	}

	if !sr.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), job) {
		t.Fatal("expected job to report but did not")
	}
}

type fakeSlackClient struct {
	messages map[string]string
}

func (fsc *fakeSlackClient) WriteMessage(text, channel string) error {
	if fsc.messages == nil {
		fsc.messages = map[string]string{}
	}
	fsc.messages[channel] = text
	return nil
}

var _ slackClient = &fakeSlackClient{}

func TestReportDefaultsToExtraRefs(t *testing.T) {
	job := &v1.ProwJob{
		Spec: v1.ProwJobSpec{
			Type:      v1.PeriodicJob,
			ExtraRefs: []v1.Refs{{Org: "org"}},
		},
		Status: v1.ProwJobStatus{
			State: v1.SuccessState,
		},
	}
	sr := slackReporter{
		config: func(r *v1.Refs) config.SlackReporter {
			if r.Org == "org" {
				return config.SlackReporter{
					JobTypesToReport:  []v1.ProwJobType{v1.PeriodicJob},
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
					Channel:           "emercengy",
					ReportTemplate:    "there you go",
				}
			}
			return config.SlackReporter{}
		},
		client: &fakeSlackClient{},
	}

	if _, _, err := sr.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), job); err != nil {
		t.Fatalf("reporting failed: %v", err)
	}
	if sr.client.(*fakeSlackClient).messages["emercengy"] != "there you go" {
		t.Errorf("expected the channel 'emergency' to contain message 'there you go' but wasn't the case, all messages: %v", sr.client.(*fakeSlackClient).messages)
	}
}
