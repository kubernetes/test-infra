/*
Copyright 2020 The Kubernetes Authors.

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

package common

import (
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io/providers"
)

// fakeProwJobFetcher is used to fetch ProwJobs in tests
type fakeProwJobFetcher struct {
	prowJob prowapi.ProwJob
}

func (j *fakeProwJobFetcher) GetProwJob(job, id string) (prowapi.ProwJob, error) {
	return j.prowJob, nil
}

func TestProwToGCS(t *testing.T) {
	type args struct {
		fetcher ProwJobFetcher
		config  config.Getter
		prowKey string
	}
	tests := []struct {
		name                string
		args                args
		wantStorageProvider string
		wantGCSKey          string
		wantErr             bool
	}{
		{
			name: "legacy gs bucket, gcs status url and gcs job url prefix",
			args: args{
				fetcher: &fakeProwJobFetcher{
					prowJob: prowapi.ProwJob{
						Spec: prowapi.ProwJobSpec{
							DecorationConfig: &prowapi.DecorationConfig{
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "kubernetes-jenkins",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
									PathStrategy: prowapi.PathStrategyLegacy,
								},
							},
						},
						Status: prowapi.ProwJobStatus{
							URL: "https://prow.k8s.io/view/gcs/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
						},
					},
				},
				config: func() *config.Config {
					return &config.Config{
						ProwConfig: config.ProwConfig{
							Plank: config.Plank{
								JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/gcs/"},
							},
						},
					}
				},
				prowKey: "ci-benchmark-microbenchmarks/1258197944759226371",
			},
			wantStorageProvider: providers.GS,
			wantGCSKey:          "kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
		},
		{
			name: "gs bucket, gcs status url and gcs job url prefix",
			args: args{
				fetcher: &fakeProwJobFetcher{
					prowJob: prowapi.ProwJob{
						Spec: prowapi.ProwJobSpec{
							DecorationConfig: &prowapi.DecorationConfig{
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "gs://kubernetes-jenkins",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
									PathStrategy: prowapi.PathStrategyLegacy,
								},
							},
						},
						Status: prowapi.ProwJobStatus{
							URL: "https://prow.k8s.io/view/gcs/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
						},
					},
				},
				config: func() *config.Config {
					return &config.Config{
						ProwConfig: config.ProwConfig{
							Plank: config.Plank{
								JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/gcs/"},
							},
						},
					}
				},
				prowKey: "ci-benchmark-microbenchmarks/1258197944759226371",
			},
			wantStorageProvider: providers.GS,
			wantGCSKey:          "kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
		},
		{
			name: "gs bucket, gs status url and gs job url prefix",
			args: args{
				fetcher: &fakeProwJobFetcher{
					prowJob: prowapi.ProwJob{
						Spec: prowapi.ProwJobSpec{
							DecorationConfig: &prowapi.DecorationConfig{
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "gs://kubernetes-jenkins",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
									PathStrategy: prowapi.PathStrategyLegacy,
								},
							},
						},
						Status: prowapi.ProwJobStatus{
							URL: "https://prow.k8s.io/view/gs/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
						},
					},
				},
				config: func() *config.Config {
					return &config.Config{
						ProwConfig: config.ProwConfig{
							Plank: config.Plank{
								JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/gs/"},
							},
						},
					}
				},
				prowKey: "ci-benchmark-microbenchmarks/1258197944759226371",
			},
			wantStorageProvider: providers.GS,
			wantGCSKey:          "kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
		},
		{
			name: "s3 bucket, s3 status url and s3 job url prefix",
			args: args{
				fetcher: &fakeProwJobFetcher{
					prowJob: prowapi.ProwJob{
						Spec: prowapi.ProwJobSpec{
							DecorationConfig: &prowapi.DecorationConfig{
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "s3://kubernetes-jenkins",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
									PathStrategy: prowapi.PathStrategyLegacy,
								},
							},
						},
						Status: prowapi.ProwJobStatus{
							URL: "https://prow.k8s.io/view/s3/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
						},
					},
				},
				config: func() *config.Config {
					return &config.Config{
						ProwConfig: config.ProwConfig{
							Plank: config.Plank{
								JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/s3/"},
							},
						},
					}
				},
				prowKey: "ci-benchmark-microbenchmarks/1258197944759226371",
			},
			wantStorageProvider: providers.S3,
			wantGCSKey:          "kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStorageProvider, gotGCSKey, err := ProwToGCS(tt.args.fetcher, tt.args.config, tt.args.prowKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProwToGCS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotStorageProvider != tt.wantStorageProvider {
				t.Errorf("ProwToGCS() gotStorageProvider = %v, wantStorageProvider %v", gotStorageProvider, tt.wantStorageProvider)
			}
			if gotGCSKey != tt.wantGCSKey {
				t.Errorf("ProwToGCS() gotGCSKey = %v, wantGCSKey %v", gotGCSKey, tt.wantGCSKey)
			}
		})
	}
}
