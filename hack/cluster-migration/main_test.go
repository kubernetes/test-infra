/*
Copyright 2023 The Kubernetes Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	cfg "k8s.io/test-infra/prow/config"
)

func TestConfigValidation_EmptyConfigPath(t *testing.T) {
	config := Config{
		configPath:    "",
		jobConfigPath: "somepath",
		repoReport:    false,
		repo:          "someRepo",
		output:        "someOutput",
	}

	err := config.validate()
	if err == nil {
		t.Error("Expected an error due to empty configPath, but got none")
	}

	if err.Error() != "--config must set" {
		t.Errorf("Expected error message '--config must set', but got: %s", err.Error())
	}
}

func TestConfigValidation_ValidConfigPath(t *testing.T) {
	config := Config{
		configPath:    "validPath",
		jobConfigPath: "somepath",
		repoReport:    false,
		repo:          "someRepo",
		output:        "someOutput",
	}

	err := config.validate()
	if err != nil {
		t.Errorf("Did not expect an error, but got: %s", err.Error())
	}
}

func TestGetJobStatus_NonDefaultCluster(t *testing.T) {
	job := cfg.JobBase{
		Cluster: "test-infra-trusted",
	}

	cluster, eligible, _ := getJobStatus(job)
	if cluster != "test-infra-trusted" || !eligible {
		t.Errorf("Expected cluster 'test-infra-trusted' and eligible true, got cluster '%s' and eligible %v", cluster, eligible)
	}
}

func TestGetJobStatus_DefaultClusterEligible(t *testing.T) {
	type testCase struct {
		name             string
		job              cfg.JobBase
		expectedCluster  string
		expectedEligible bool
	}
	jobs := []testCase{
		{
			name: "Test Case: Job is in the default cluster",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job is in the eks-prow-build-cluster cluster",
			job: cfg.JobBase{
				Cluster: "eks-prow-build-cluster",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
				},
			},
			expectedCluster:  "eks-prow-build-cluster",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job is in the default cluster and has a cred label",
			job: cfg.JobBase{
				Cluster: "default",
				Labels: map[string]string{
					"cred": "someValue",
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job has an allowed cred label",
			job: cfg.JobBase{
				Cluster: "default",
				Labels: map[string]string{
					"preset-aws-credential": "someValue",
					"preset-aws-ssh":        "someValue",
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job container environment is derived from a secret",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Env: []v1.EnvVar{
								{
									Name: "someVariable",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											Key: "someSecret",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job container environment is derived from an allowed secret",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Env: []v1.EnvVar{
								{
									Name: "someVariable",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											Key: "aws-ssh-key-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Jobs container environment has \"cred\" in the name",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Env: []v1.EnvVar{
								{
									Name: "somecred",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Jobs container environment is an allowed variable",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Env: []v1.EnvVar{
								{
									Name: "AWS_SHARED_CREDENTIALS_FILE",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Jobs container environment has \"Cred\" in the name",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Env: []v1.EnvVar{
								{
									Name: "someCred",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Jobs container arguments don't contain any disallowed words",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Args: []string{
								"test",
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Jobs container arguments contain \"gcp\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Args: []string{
								"--gcp-zone=us-central1-f",
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job container command doesn't contain disallowed words",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Command: []string{
								"someCommand",
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job container command contains \"gcp\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							Command: []string{
								"someCommand --gcp-zone=us-central1-f",
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job container volume mount contains \"Secret\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							VolumeMounts: []v1.VolumeMount{
								{
									Name: "someSecret",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job container volume mount contains \"secret\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							VolumeMounts: []v1.VolumeMount{
								{
									Name: "somesecret",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job container volume mount contains \"cred\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
							VolumeMounts: []v1.VolumeMount{
								{
									Name: "somecred",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job volume contains \"cred\"",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "someCred",
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job volume contains allowed volume name",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "aws-cred",
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job volume is derived from unapproved secret",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "someVolume",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "someSecret",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: false,
		},
		//
		{
			name: "Test Case: Job volume is derived from approved secret",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "someVolume",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "ssh-key-secret",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
		//
		{
			name: "Test Case: Job volume is derived from approved secret",
			job: cfg.JobBase{
				Cluster: "default",
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "someContainer",
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "someVolume",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: "service-account",
								},
							},
						},
					},
				},
			},
			expectedCluster:  "",
			expectedEligible: true,
		},
	}

	for _, job := range jobs {
		cluster, eligible, _ := getJobStatus(job.job)
		if cluster != job.expectedCluster || eligible != job.expectedEligible {
			t.Errorf("%v: Expected cluster '%s' and eligible %v, got cluster '%s' and eligible %v", job.name, job.expectedCluster, job.expectedEligible, cluster, eligible)
		}
	}
}

func TestGetPercentage(t *testing.T) {
	tests := []struct {
		int1, int2 int
		expected   float64
	}{
		{10, 100, 10},
		{5, 10, 50},
		{0, 10, 0},
		{10, 0, 100}, // Ensure handling division by zero
	}

	for _, test := range tests {
		result := getPercentage(test.int1, test.int2)
		if result != test.expected {
			t.Errorf("For inputs %d and %d, expected %.2f but got %.2f", test.int1, test.int2, test.expected, result)
		}
	}
}

func TestPrintPercentage(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{10.5678, "10.57%"},
		{50, "50.00%"},
		{0, "0.00%"},
	}

	for _, test := range tests {
		result := printPercentage(test.input)
		if result != test.expected {
			t.Errorf("For input %.4f, expected '%s' but got '%s'", test.input, test.expected, result)
		}
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		words    []string
		expected bool
	}{
		{"hello world", []string{"hello", "test"}, true},
		{"hello world", []string{"test", "sample"}, false},
		{"HELLO WORLD", []string{"hello"}, true}, // Ensure case-insensitivity
	}

	for _, test := range tests {
		result := containsAny(test.s, test.words)
		if result != test.expected {
			t.Errorf("For input '%s' with words %v, expected %v but got %v", test.s, test.words, test.expected, result)
		}
	}
}
