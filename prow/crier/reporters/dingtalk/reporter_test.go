package dingtalk

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func TestShouldReport(t *testing.T) {
	boolPtr := func(b bool) *bool {
		return &b
	}
	testCases := []struct {
		name     string
		config   config.DingTalkReporter
		pj       *v1.ProwJob
		expected bool
	}{
		{
			name: "Presubmit Job should report",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PresubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
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
			name: "Wrong job type  should not report",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
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
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
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
			name: "Successful Job with report:false should not report",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
					Report:            boolPtr(false),
				},
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
			name: "Successful Job with report:true should report",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
					Report:            boolPtr(true),
				},
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
			// Note: this is impossible to hit, as roundtrip with `omitempty`
			// would never result in empty slice.
			name: "Empty job config settings negate global",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{JobStatesToReport: []v1.ProwJobState{}},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: false,
		},
		{
			name: "Nil job config settings does not negate global",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{JobStatesToReport: nil},
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expected: true,
		},
		{
			name: "Successful Job should not report",
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.PendingState},
				},
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
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{Token: "some-token"},
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
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{
							Token:             "some-token",
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
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{Token: "some-token"},
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
			config: config.DingTalkReporter{
				JobTypesToReport: []v1.ProwJobType{},
				DingTalkReporterConfig: v1.DingTalkReporterConfig{
					JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
				},
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{
							Token:             "some-token",
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
			config: config.DingTalkReporter{},
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
		cfgGetter := func(*v1.Refs) config.DingTalkReporter {
			return tc.config
		}
		t.Run(tc.name, func(t *testing.T) {
			reporter := &dingTalkReporter{
				config: cfgGetter,
			}

			if result := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj); result != tc.expected {
				t.Errorf("expected result to be %t but was %t", tc.expected, result)
			}
		})
	}
}

func TestReloadsConfig(t *testing.T) {
	cfg := config.DingTalkReporter{}
	cfgGetter := func(*v1.Refs) config.DingTalkReporter {
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

	reporter := &dingTalkReporter{
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

func TestUsesTokenOverrideFromJob(t *testing.T) {
	testCases := []struct {
		name          string
		config        func() config.Config
		pj            *v1.ProwJob
		wantToken     string
		emptyExpected bool
	}{
		{
			name: "No job-level config, use global default",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"*": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "global-default",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
					},
				}
			},
			pj:        &v1.ProwJob{Spec: v1.ProwJobSpec{}},
			wantToken: "global-default",
		},
		{
			name: "org/repo for ref exists in config, use it",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"*": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "global-default",
						},
					},
					"istio/proxy": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-repo-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
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
			wantToken: "org-repo-config",
		},
		{
			name: "org for ref exists in config, use it",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"*": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "global-default",
						},
					},
					"istio": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
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
			wantToken: "org-config",
		},
		{
			name: "org/repo takes precedence over org",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"*": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "global-default",
						},
					},
					"istio": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-config",
						},
					},
					"istio/proxy": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-repo-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
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
			wantToken: "org-repo-config",
		},
		{
			name: "Job-level config present, use it",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"*": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "global-default",
						},
					},
					"istio": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-config",
						},
					},
					"istio/proxy": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-repo-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
					},
				}
			},
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					ReporterConfig: &v1.ReporterConfig{
						DingTalk: &v1.DingTalkReporterConfig{
							Token: "team-a",
						},
					},
				},
			},
			wantToken: "team-a",
		},
		{
			name: "No matching dingTalk config",
			config: func() config.Config {
				dingTalkCfg := map[string]config.DingTalkReporter{
					"istio": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-config",
						},
					},
					"istio/proxy": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-repo-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
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
				dingTalkCfg := map[string]config.DingTalkReporter{
					"istio/proxy": {
						DingTalkReporterConfig: v1.DingTalkReporterConfig{
							Token: "org-repo-config",
						},
					},
				}
				return config.Config{
					ProwConfig: config.ProwConfig{
						DingTalkReporterConfigs: dingTalkCfg,
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
			wantToken: "org-repo-config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfgGetter := func(refs *v1.Refs) config.DingTalkReporter {
				return tc.config().DingTalkReporterConfigs.GetDingTalkReporter(refs)
			}
			sr := dingTalkReporter{
				config: cfgGetter,
			}

			prowSlackCfg, jobDingTalkCfg := sr.getConfig(tc.pj)
			jobDingTalkCfg = jobDingTalkCfg.ApplyDefault(&prowSlackCfg.DingTalkReporterConfig)
			gotToken := jobDingTalkCfg.Token
			if gotToken != tc.wantToken {
				t.Fatalf("Expected token: %q, got: %q", tc.wantToken, gotToken)
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
	sr := dingTalkReporter{
		config: func(r *v1.Refs) config.DingTalkReporter {
			if r.Org == "org" {
				return config.DingTalkReporter{
					JobTypesToReport: []v1.ProwJobType{v1.PeriodicJob},
					DingTalkReporterConfig: v1.DingTalkReporterConfig{
						JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
					},
				}
			}
			return config.DingTalkReporter{}
		},
	}

	if !sr.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), job) {
		t.Fatal("expected job to report but did not")
	}
}

type fakeDingTalkClient struct {
	messages map[string]string
}

func (fsc *fakeDingTalkClient) WriteMessage(msg, token string) error {
	if fsc.messages == nil {
		fsc.messages = map[string]string{}
	}
	fsc.messages[token] = msg
	return nil
}

var _ dingTalkClient = &fakeDingTalkClient{}

func TestReportDefaultsToExtraRefs(t *testing.T) {
	job := &v1.ProwJob{
		Spec: v1.ProwJobSpec{
			Type:      v1.PeriodicJob,
			ExtraRefs: []v1.Refs{{Org: "org", Repo: "repo"}},
		},
		Status: v1.ProwJobStatus{
			State: v1.SuccessState,
		},
	}
	fsc := &fakeDingTalkClient{}
	sr := dingTalkReporter{
		config: func(r *v1.Refs) config.DingTalkReporter {
			if r.Org == "org" {
				return config.DingTalkReporter{
					JobTypesToReport: []v1.ProwJobType{v1.PeriodicJob},
					DingTalkReporterConfig: v1.DingTalkReporterConfig{
						JobStatesToReport: []v1.ProwJobState{v1.SuccessState},
						Token:             "emercengy",
						ReportTemplate: `{{ $repo := "" }}{{with .Spec.Refs}}{{$repo = .Repo}}{{end}}{{if eq $repo ""}}{{with index .Spec.ExtraRefs 0}}{{$repo = .Repo}}{{end}}{{end}}## Repo: {{ $repo }}
---
- Job: {{.Spec.Job}}
- Type: {{.Spec.Type}}
- State: {{if eq .Status.State "triggered"}}<font color="orange">**{{.Status.State}}**</font>{{end}}{{if eq .Status.State "pending"}}<font color="yellow">**{{.Status.State}}**</font>{{end}}{{if eq .Status.State "success"}}<font color="green">**{{.Status.State}}**</font>{{end}}{{if eq .Status.State "failure"}}<font color="red">**{{.Status.State}}**</font>{{end}}{{if eq .Status.State "aborted"}}<font color="gray">**{{.Status.State}}**</font>{{end}}{{if eq .Status.State "error"}}<font color="red">**{{.Status.State}}**</font>{{end}}
- Log: [View logs]({{.Status.URL}})`,
					},
				}
			}
			return config.DingTalkReporter{}
		},
		client: fsc,
	}
	wantMessage := `## Repo: repo
---
- Job: 
- Type: periodic
- State: <font color="green">**success**</font>
- Log: [View logs]()`
	if _, _, err := sr.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), job); err != nil {
		t.Fatalf("reporting failed: %v", err)
	}
	if fsc.messages["emercengy"] != wantMessage {
		t.Errorf("expected the token 'emergency' to contain message 'there you go' but wasn't the case, all messages: %v", fsc.messages)
	}
}
