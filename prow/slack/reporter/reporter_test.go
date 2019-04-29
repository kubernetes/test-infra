package reporter

import (
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reporter := &slackReporter{
				config: tc.config,
				logger: logrus.NewEntry(&logrus.Logger{}),
			}

			if result := reporter.ShouldReport(tc.pj); result != tc.expected {
				t.Errorf("expected result to be %t but was %t", tc.expected, result)
			}
		})
	}
}
