/*
Copyright 2017 The Kubernetes Authors.

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

package analysis

import (
	"context"
	"fmt"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
	"regexp"
	"testing"
)

type storageBucketTester struct {
	listAllResponse    []string
	listAllError       error
	readObjectResponse []byte
	readObjectError    error
}

func (sbt storageBucketTester) listAll(ctx context.Context, prefix string) ([]string, error) {
	return sbt.listAllResponse, sbt.listAllError
}

func (sbt storageBucketTester) readObject(ctx context.Context, key string) ([]byte, error) {
	return sbt.readObjectResponse, sbt.readObjectError
}

func initRiskAnalysis(cfg config.Config, filename *regexp.Regexp, sbt storageBucketTester) (*RiskAnalysis, error) {

	var sbi = func(bucketName, storageProvider string, config *config.Config, opener pkgio.Opener) (storageBucket, error) {
		return sbt, nil
	}

	return &RiskAnalysis{
		config: &cfg,
		//opener: 	opener,
		filename:          filename,
		bucketInitializer: sbi,
		analysisPath:      DefaultAnalysisPath,
	}, nil
}

func TestArtifacts(t *testing.T) {

	var testcases = []struct {
		name         string
		risk         string
		expectedRisk string
	}{
		{
			name:         "unknown",
			risk:         `{"OverallRisk": {"Level": {"Name": "Unknown"}}}`,
			expectedRisk: "unknown",
		},
		{
			name:         "high",
			risk:         `{"OverallRisk": {"Level": {"Name": "hiGh"}}}`,
			expectedRisk: "high",
		},
		{
			name:         "missing",
			risk:         `{"OverallRiskWrong": {"Level": {"Name": "Unknown"}}}`,
			expectedRisk: "unknown",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			raFn := func(pj prowapi.ProwJob) (ProwJobFailureRisk, error) {

				raCfg := config.Config{
					ProwConfig: config.ProwConfig{
						Plank: config.Plank{
							JobURLPrefixConfig: map[string]string{"*": "https://prow.ci.test.org/view"},
						},
						Deck: config.Deck{},
					},
				}

				unknown := tc.risk

				sbt := &storageBucketTester{readObjectResponse: []byte(unknown), readObjectError: nil, listAllResponse: []string{fmt.Sprintf("%s.json", TestFailureSummaryFilePrefix)}}

				ra, err := initRiskAnalysis(raCfg, GetDefaultRiskAnalysisSummaryFile(), *sbt)

				if err != nil {
					return ProwJobFailureRiskUnknown, err
				}

				return ra.ProwJobFailureRiskAnalysisDefault(pj)
			}

			job := &prowapi.ProwJob{Spec: prowapi.ProwJobSpec{Job: "testJob"}, Status: prowapi.ProwJobStatus{BuildID: "99023", URL: "https://prow.ci.test.org/view/gs/ci-test-bucket/pr-logs/pull/27486/pull-ci-test-master-e2e-aws-ovn-single-node-upgrade/1584599988365406208"}}
			risk, err := raFn(*job)

			if err != nil {
				t.Errorf("Error initializing risk analysis: %v", err)
			}

			if risk != ProwJobFailureRisk(tc.expectedRisk) {
				t.Errorf("Expected risk: %s, received: %s", string(ProwJobFailureRiskUnknown), string(risk))
			}

		})
	}
}
