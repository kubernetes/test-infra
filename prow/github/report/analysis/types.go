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
	"regexp"
	"strings"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
)

type ProwJobFailureRisk string

// Various job failure risk levels.
const (
	// HighRisk means the job has a high (e.g. > 98%) success rate and a failure indicates a higher chance of regression.
	ProwJobFailureRiskHigh ProwJobFailureRisk = "high"
	// MediumRisk means the job is regularly successful (e.g. > 80%) and a failure indicates potential regression
	ProwJobFailureRiskMedium ProwJobFailureRisk = "medium"
	// LowRisk means the job does not have a consistent success profile (e.g. < 80%) so the failure alone can not be relied on as indicator for regression
	ProwJobFailureRiskLow ProwJobFailureRisk = "low"
	// UnknownRisk means the risk analysis for this job could not determine a risk level
	ProwJobFailureRiskUnknown ProwJobFailureRisk = "unknown"
)

func FindProwJobFailureRiskLevel(in string) ProwJobFailureRisk {

	switch strings.ToLower(in) {
	case string(ProwJobFailureRiskHigh):
		return ProwJobFailureRiskHigh
	case string(ProwJobFailureRiskMedium):
		return ProwJobFailureRiskMedium
	case string(ProwJobFailureRiskLow):
		return ProwJobFailureRiskLow
	default:
		return ProwJobFailureRiskUnknown
	}

}

type ProwJobFailureRiskAnalysis func(pj prowapi.ProwJob) (ProwJobFailureRisk, error)

type StorageBucketInitializer func(bucketName, storageProvider string, config *config.Config, opener pkgio.Opener) (storageBucket, error)

type StaticProwJobFetcher struct {
	prowJob prowapi.ProwJob
}

func (j *StaticProwJobFetcher) GetProwJob(job, id string) (prowapi.ProwJob, error) {
	return j.prowJob, nil
}

type RiskAnalysis struct {
	config            *config.Config
	opener            pkgio.Opener
	filename          *regexp.Regexp
	bucketInitializer StorageBucketInitializer
	analysisPath      []string
}

type RiskAnalysisResult struct {
	Name string
	Data map[string]any
}

// storageBucket is an abstraction for unit testing
type storageBucket interface {
	listAll(ctx context.Context, prefix string) ([]string, error)
	readObject(ctx context.Context, key string) ([]byte, error)
}

// blobStorageBucket is our real implementation of storageBucket.
// Use `newBlobStorageBucket` to instantiate (includes bucket-level validation).
type blobStorageBucket struct {
	name            string
	storageProvider string
	pkgio.Opener
}
