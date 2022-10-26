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
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	pkgio "k8s.io/test-infra/prow/io"
	lenses "k8s.io/test-infra/prow/spyglass/lenses/common"
)

const TestFailureSummaryFilePrefix = "risk-analysis"

var DefaultAnalysisPath = []string{"OverallRisk", "Level", "Name"}

func CreateRiskAnalysisFn(cfg config.Config, storage prowflagutil.StorageClientOptions) ProwJobFailureRiskAnalysis {
	var raFn ProwJobFailureRiskAnalysis
	{
		raFn = func(pj prowapi.ProwJob) (ProwJobFailureRisk, error) {
			raCfg := config.Config{
				JobConfig:  cfg.JobConfig,
				ProwConfig: cfg.ProwConfig,
			}
			asf := GetDefaultRiskAnalysisSummaryFile()
			if len(cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryLocator) > 0 {
				r, err := regexp.Compile(cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryLocator)

				if err != nil {
					logrus.WithError(err).Errorf("Error compiling analyze-test-failure-summary-locator: %s.  Using default", cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryLocator)
				} else {
					asf = r
				}
			}

			ap := DefaultAnalysisPath
			if len(cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryValuePath) > 0 {
				p := strings.Split(cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryValuePath, ",")
				if len(p) < 1 {
					logrus.Errorf("Error parsing analyze-test-failure-summary-path: %s.  Using default", cfg.GitHubReporter.TestFailureRiskAnalysis.SummaryValuePath)
				} else {
					ap = p
				}
			}
			ra, err := InitRiskAnalysis(raCfg, storage, asf, ap)

			if err != nil {
				return ProwJobFailureRiskUnknown, err
			}

			return ra.ProwJobFailureRiskAnalysisDefault(pj)
		}
	}

	return raFn
}

func GetDefaultRiskAnalysisSummaryFile() *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf("%s.*.json", TestFailureSummaryFilePrefix))
}

func InitRiskAnalysis(cfg config.Config, storage prowflagutil.StorageClientOptions, filename *regexp.Regexp, analysisPath []string) (*RiskAnalysis, error) {
	ctx := context.TODO()
	opener, err := pkgio.NewOpener(ctx, storage.GCSCredentialsFile, storage.S3CredentialsFile)
	if err != nil {
		logrus.WithError(err).Error("Error creating opener")
		return &RiskAnalysis{}, err
	}

	return &RiskAnalysis{
		config:            &cfg,
		opener:            opener,
		filename:          filename,
		bucketInitializer: newBlobStorageBucket,
		analysisPath:      analysisPath,
	}, nil
}

func (ra RiskAnalysis) ProwJobFailureRiskAnalysisDefault(pj prowapi.ProwJob) (ProwJobFailureRisk, error) {

	// doesn't really matter since we are using staticFetcher
	prowKey := pj.Spec.Job + "/" + pj.Status.BuildID

	staticFetcher := &StaticProwJobFetcher{prowJob: pj}
	configGetter := func() *config.Config {
		return ra.config
	}
	// we have to get the bucket name from the gcsKey so we can get the StorageProvider from there as well
	_, gcsKey, err := lenses.ProwToGCS(staticFetcher, configGetter, prowKey)

	if err != nil {
		logrus.WithError(err).Error("Error converting prow to gcs")
		return ProwJobFailureRiskUnknown, err
	}

	sp, bucketName, root, err := parseGCSKey(gcsKey)

	if err != nil {
		return ProwJobFailureRiskUnknown, err
	}

	riskAnalysisResult := ra.findRiskAnalysisData(sp, bucketName, root, ra.filename, ra.config, ra.opener)

	if len(riskAnalysisResult) > 0 {

		// for now we take the first one
		return extractRiskAnalysis(riskAnalysisResult[0].Data, ra.analysisPath), nil
	}

	// default
	return ProwJobFailureRiskUnknown, nil
}

func extractRiskAnalysis(analysis map[string]any, path []string) ProwJobFailureRisk {

	if len(path) > 0 {
		value := analysis
		for i, key := range path {

			if value == nil || value[key] == nil {
				break
			}

			if i < (len(path) - 1) {
				value = value[key].(map[string]any)
			} else {
				return FindProwJobFailureRiskLevel(value[key].(string))
			}
		}
	}

	return ProwJobFailureRiskUnknown
}

func (ra RiskAnalysis) findRiskAnalysisData(storageProvider, bucketName, root string, filename *regexp.Regexp, config *config.Config, opener pkgio.Opener) []RiskAnalysisResult {

	bucket, err := ra.bucketInitializer(bucketName, storageProvider, config, opener)

	if err != nil {
		logrus.WithError(err).Errorf("Error getting bucket: %s", bucketName)
		return nil
	}

	ctx := context.TODO()
	files, err := bucket.listAll(ctx, root)

	if err != nil {
		logrus.WithError(err).Errorf("Error getting file list for path: %s/%s", bucketName, root)
		return nil
	}

	for _, name := range files {

		// we match on the first found filename
		// technically there could be multiples
		// we support returning array but quit after the first one for now
		if filename.MatchString(name) {
			var data map[string]any
			err = readJSON(ctx, bucket, name, &data)

			// if we had an error keep looking, or bail?
			if err != nil {
				logrus.WithError(err).Errorf("Error reading file: %s/%s", bucketName, name)
			} else {
				return []RiskAnalysisResult{{Name: name, Data: data}}
			}
		}
	}

	// we didn't find any  ...
	logrus.Warn("No match for risk analysis data.")
	return nil
}

// newBlobStorageBucket validates the bucketName and returns a new instance of blobStorageBucket.
func newBlobStorageBucket(bucketName, storageProvider string, config *config.Config, opener pkgio.Opener) (storageBucket, error) {
	if err := config.ValidateStorageBucket(bucketName); err != nil {
		return blobStorageBucket{}, fmt.Errorf("could not instantiate storage bucket: %w", err)
	}
	return blobStorageBucket{bucketName, storageProvider, opener}, nil
}

func (bucket blobStorageBucket) readObject(ctx context.Context, key string) ([]byte, error) {
	u := url.URL{
		Scheme: bucket.storageProvider,
		Host:   bucket.name,
		Path:   key,
	}
	rc, err := bucket.Opener.Reader(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("creating reader for object %s: %w", key, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// reads specified JSON file in to `data`
func readJSON(ctx context.Context, bucket storageBucket, key string, data interface{}) error {
	rawData, err := bucket.readObject(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", key, err)
	}
	err = json.Unmarshal(rawData, &data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", key, err)
	}
	return nil
}

// Lists all keys with given prefix.
func (bucket blobStorageBucket) listAll(ctx context.Context, prefix string) ([]string, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	it, err := bucket.Opener.Iterator(ctx, fmt.Sprintf("%s://%s/%s", bucket.storageProvider, bucket.name, prefix), "")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	for {
		attrs, err := it.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return keys, err
		}
		keys = append(keys, attrs.Name)
	}
	return keys, nil
}

func parseGCSKey(gcsKey string) (storageProvider, bucketName, root string, err error) {
	// remove any leading '/'
	p := strings.TrimPrefix(gcsKey, "/")
	// examples for p:
	// "/gs/ci-test/pr-logs/pull/27486/pull-ci-test-origin-master-e2e-aws-ovn-single-node-upgrade/1584099588365406208"

	s := strings.SplitN(p, "/", 3)
	if len(s) < 3 {
		err = fmt.Errorf("invalid path (expected either <storage-type>/<bucket-name>/<storage-path>): %v", gcsKey)
		return
	}
	storageProvider = s[0]
	bucketName = s[1]
	root = s[2] // `root` is the root "directory" prefix for this job's results

	if bucketName == "" {
		err = fmt.Errorf("missing bucket name: %v", gcsKey)
		return
	}
	if root == "" {
		err = fmt.Errorf("invalid path for job: %v", gcsKey)
		return
	}

	return
}
