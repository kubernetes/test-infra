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

package kube

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/rest"
)

func TestPluggableInClusterConfigLoader(t *testing.T) {
	var testCases = []struct {
		name   string
		loader func() (*rest.Config, error)

		expected        map[string]rest.Config
		expectedDefault *string
		expectedErr     bool
	}{
		{
			name:            "no error loading leads to a correct config",
			loader:          func() (*rest.Config, error) { return &rest.Config{Host: "foobar"}, nil },
			expected:        map[string]rest.Config{*inClusterContext(): {Host: "foobar"}},
			expectedDefault: nil,
			expectedErr:     false,
		},
		{
			name:            "error loading leads to no config",
			loader:          func() (*rest.Config, error) { return nil, errors.New("oops") },
			expected:        map[string]rest.Config{},
			expectedDefault: nil,
			expectedErr:     false, // only will emit a warning
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configurations, defaultContext, err := pluggableInClusterConfigLoader(testCase.loader)()
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !reflect.DeepEqual(configurations, testCase.expected) {
				t.Errorf("%s: got incorrect cluster configurations: %s", testCase.name, diff.ObjectReflectDiff(configurations, testCase.expected))
			}
			if defaultContext != testCase.expectedDefault {
				t.Errorf("%s: got incorrect default context: %s", testCase.name, diff.ObjectReflectDiff(defaultContext, testCase.expectedDefault))
			}
		})
	}
}

func strPointer(input string) *string {
	return &input
}

func TestKubeconfigConfigLoader(t *testing.T) {
	var testCases = []struct {
		name         string
		explicitPath bool
		kubeconfig   string

		expected        map[string]rest.Config
		expectedDefault *string
		expectedErr     bool
	}{
		{
			name:         "kubeconfig loaded from explicit flag and current context as default",
			explicitPath: true,
			kubeconfig: `apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    server: https://some.cluster.com:443
  name: some-cluster-com:443
users:
- name: some-user/some-cluster-com:443
  user:
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3
contexts:
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: some-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/some-user
current-context: some-namespace/some-cluster-com:443/some-user
`,
			expected: map[string]rest.Config{
				"some-namespace/some-cluster-com:443/some-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3",
				},
			},
			expectedDefault: strPointer("some-namespace/some-cluster-com:443/some-user"),
			expectedErr:     false,
		},
		{
			name:         "complex kubeconfig loaded from explicit flag and current context as default has an entry per context",
			explicitPath: true,
			kubeconfig: `apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    server: https://some.cluster.com:443
  name: some-cluster-com:443
users:
- name: some-user/some-cluster-com:443
  user:
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3
- name: second-user/some-cluster-com:443
  user:
    token: MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia
- name: third-user/some-cluster-com:443
  user:
    token: 3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1l
contexts:
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: some-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/some-user
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: second-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/second-user
- context:
    cluster: some-cluster-com:443
    namespace: other-namespace
    user: third-user/some-cluster-com:443
  name: other-namespace/some-cluster-com:443/third-user
current-context: some-namespace/some-cluster-com:443/some-user
`,
			expected: map[string]rest.Config{
				"some-namespace/some-cluster-com:443/some-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3",
				},
				"some-namespace/some-cluster-com:443/second-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia",
				},
				"other-namespace/some-cluster-com:443/third-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1l",
				},
			},
			expectedDefault: strPointer("some-namespace/some-cluster-com:443/some-user"),
			expectedErr:     false,
		},
		{
			name: "kubeconfig loaded from env and current context as default",
			kubeconfig: `apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    server: https://some.cluster.com:443
  name: some-cluster-com:443
users:
- name: some-user/some-cluster-com:443
  user:
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3
contexts:
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: some-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/some-user
current-context: some-namespace/some-cluster-com:443/some-user
`,
			expected: map[string]rest.Config{
				"some-namespace/some-cluster-com:443/some-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3",
				},
			},
			expectedDefault: strPointer("some-namespace/some-cluster-com:443/some-user"),
			expectedErr:     false,
		},
		{
			name:         "kubeconfig loaded from explicit flag with no current context has no default",
			explicitPath: true,
			kubeconfig: `apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    server: https://some.cluster.com:443
  name: some-cluster-com:443
users:
- name: some-user/some-cluster-com:443
  user:
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3
contexts:
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: some-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/some-user
current-context: ""
`,
			expected: map[string]rest.Config{
				"some-namespace/some-cluster-com:443/some-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3",
				},
			},
			expectedDefault: nil,
			expectedErr:     false,
		},
		{
			name:         "kubeconfig loaded from explicit flag and invalid current context has no default",
			explicitPath: true,
			kubeconfig: `apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    server: https://some.cluster.com:443
  name: some-cluster-com:443
users:
- name: some-user/some-cluster-com:443
  user:
    token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3
contexts:
- context:
    cluster: some-cluster-com:443
    namespace: some-namespace
    user: some-user/some-cluster-com:443
  name: some-namespace/some-cluster-com:443/some-user
current-context: something-invalid
`,
			expected: map[string]rest.Config{
				"some-namespace/some-cluster-com:443/some-user": {
					Host:        "https://some.cluster.com:443",
					BearerToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3",
				},
			},
			expectedDefault: nil,
			expectedErr:     false,
		},
		{
			name:         "kubeconfig loaded from explicit flag with invalid content causes an error",
			explicitPath: true,
			kubeconfig: `apiVersion: v123
kind: Config`,
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
		{
			name: "kubeconfig loaded from env with invalid content causes a warning",
			kubeconfig: `apiVersion: v123
kind: Config`,
			expected:        map[string]rest.Config{},
			expectedDefault: nil,
			expectedErr:     false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			kubeconfig, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatalf("%s: could not create kubeconfig file: %v", testCase.name, err)
			}
			defer func() {
				if err := os.Remove(kubeconfig.Name()); err != nil {
					t.Fatalf("%s: failed to clean up temp file: %v", testCase.name, err)
				}
			}()
			if _, err := kubeconfig.WriteString(testCase.kubeconfig); err != nil {
				t.Fatalf("%s: could not populate kubeconfig file: %v", testCase.name, err)
			}

			// we either read this explicitly through the flag or implictly through the env
			var name string
			if testCase.explicitPath {
				name = kubeconfig.Name()
			} else {
				if err := os.Setenv("KUBECONFIG", kubeconfig.Name()); err != nil {
					t.Fatalf("%s: could not populate $KUBECONFIG env: %v", testCase.name, err)
				}
			}
			configurations, defaultContext, err := kubeconfigConfigLoader(name)()
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !reflect.DeepEqual(configurations, testCase.expected) {
				t.Errorf("%s: got incorrect cluster configurations: %s", testCase.name, diff.ObjectReflectDiff(configurations, testCase.expected))
			}
			if !reflect.DeepEqual(defaultContext, testCase.expectedDefault) {
				t.Errorf("%s: got incorrect default context: %s", testCase.name, diff.ObjectReflectDiff(defaultContext, testCase.expectedDefault))
			}
		})
	}
}

func TestBuildClusterConfigLoader(t *testing.T) {
	var testCases = []struct {
		name         string
		buildCluster string

		expected        map[string]rest.Config
		expectedDefault *string
		expectedErr     bool
	}{
		{
			name: "implicit build cluster loads fine and sets no default",
			buildCluster: `endpoint: https://some.cluster.com:443
clientCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNIakNDQWNnQ0FRRXdEUVlKS29aSWh2Y05BUUVMQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWUQKVlFRSURBcFhZWE5vYVc1bmRHOXVNUkF3RGdZRFZRUUhEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZwpTR0YwTVJJd0VBWURWUVFMREFsUGNHVnVVMmhwWm5ReEdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqCmIyMHhJakFnQmdrcWhraUc5dzBCQ1FFV0UzTnJkWHB1WlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXcKTWpNME9UQXlXaGNOTWpBd01qRXdNak0wT1RBeVdqQ0JtVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTQpDbGRoYzJocGJtZDBiMjR4RURBT0JnTlZCQWNNQjFObFlYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4CkVqQVFCZ05WQkFzTUNVOXdaVzVUYUdsbWRERVpNQmNHQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWkKTUNBR0NTcUdTSWIzRFFFSkFSWVRjMnQxZW01bGRITkFjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQgpBUVVBQTBzQU1FZ0NRUUMyOThDSXBJVzFDUUZ1clczWVNjTVNMSGx5V1JZNXozY0JuSXRFT1ErMWZuLzU3NmZtCk5Ha3pXemxKcXVPWFNVMlNtdytrUFlha3l6ZHFCRHZBRzBiakFnTUJBQUV3RFFZSktvWklodmNOQVFFTEJRQUQKUVFDUUZMYVkvRUpNRkVCQllGRkIrUUhSOXdMcVlaSW0ydEpKd0grSXNEVGIxd0xnWE5JSHBpbjZ1SzIxdmJwbgo5YXU4alBUUWRXSGFrQTdKSUNLSytxYzUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
clientKey: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpQcm9jLVR5cGU6IDQsRU5DUllQVEVECkRFSy1JbmZvOiBERVMtRURFMy1DQkMsMDZFMENEMkFFNTI0NEMwQwoKZ2JmT3JkTzV1ZjZuNUdkbnVidUNnSkhjSkxGQkFiUWMzRE5CaXkxK2RkTHhzeFR6VU1mdllHQ21SN01hY2RBQwpsTGJBajlqcGd4QTFweEFpaGxyK2RyTjNsTExkMWt6Z1JER3UvcU13ZjlRK1ErY29XZmZiOVhJbFM1NFhPUnRUCnN4VVI4SFVXeExLcVRIZXZJT2loYWFlQVA5T05MVHZrOGVDaXhwK0NZS0hmWGhHMkJwNHNkUmUxK3NtMjlKZVMKTFJiZVF2SFdhM2pPRG5VZ2gzbUJnRlBWWHptN2xhYkdJTHkyTUU5ZWIrRElYd09xZjBOVlNvNlFkVUoyT2pFRwo2UWMrWS8wc3NNdDVwbm1zZG9obVl5QTh6Q0plcDlQY0g3RlpmZzB1dmFjT2RCcER6eDU2Vng5OFJETHZHbC9MCndPdjBsa3EvcUd3czQwdkZETmdkNmFvUWMxU1pMWG9kY293UDhDZEJCQlY4REFxNkFPUU5wZVN2UDF0OTJKVXEKbVhFa1FlaEFvRE1uZlBMSjFxWkIrMGY3VnFhVGpiRVRKeWVXNjZlaW84OD0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K
clusterCaCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNpekNDQWpXZ0F3SUJBZ0lVQy9NalBPalFGTzhRVWR4bHNqcTBSWnBzVE9Zd0RRWUpLb1pJaHZjTkFRRUwKQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJREFwWFlYTm9hVzVuZEc5dU1SQXdEZ1lEVlFRSApEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZ1NHRjBNUkl3RUFZRFZRUUxEQWxQY0dWdVUyaHBablF4CkdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqYjIweElqQWdCZ2txaGtpRzl3MEJDUUVXRTNOcmRYcHUKWlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXdNak0wTnpJMVdoY05NakF3TWpFd01qTTBOekkxV2pDQgptVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTUNsZGhjMmhwYm1kMGIyNHhFREFPQmdOVkJBY01CMU5sCllYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4RWpBUUJnTlZCQXNNQ1U5d1pXNVRhR2xtZERFWk1CY0cKQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWlNQ0FHQ1NxR1NJYjNEUUVKQVJZVGMydDFlbTVsZEhOQQpjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQkFRVUFBMHNBTUVnQ1FRQ21MRDQyTk1sVDgxenlYSSsxCmVvazlEQ0JPN3JYQ0hxTk1mcTRkb0l3OGdJMW5qU1dYa242eU9YTHloN2xCTUovYnhvRWxTYm1HOVVqL3NjRFIKYUlCVkFnTUJBQUdqVXpCUk1CMEdBMVVkRGdRV0JCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFmQmdOVgpIU01FR0RBV2dCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQTBHCkNTcUdTSWIzRFFFQkN3VUFBMEVBb0J6aFpsRkNQRmRwdHBmM004QlYwVEkySWdtQWQzam5aVXUzb2ZubGErdXAKL1hSY1FwS0pUL3VKdnZmSytiOEJRU1VlRTJWRk1aMEJORDJFcmVaY1pRPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
`,
			expected: map[string]rest.Config{
				"default": {
					Host: "https://some.cluster.com:443",
					TLSClientConfig: rest.TLSClientConfig{
						CertData: []byte(`-----BEGIN CERTIFICATE-----
MIICHjCCAcgCAQEwDQYJKoZIhvcNAQELBQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYD
VQQIDApXYXNoaW5ndG9uMRAwDgYDVQQHDAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQg
SGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQxGTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5j
b20xIjAgBgkqhkiG9w0BCQEWE3NrdXpuZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEw
MjM0OTAyWhcNMjAwMjEwMjM0OTAyWjCBmTELMAkGA1UEBhMCVVMxEzARBgNVBAgM
Cldhc2hpbmd0b24xEDAOBgNVBAcMB1NlYXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQx
EjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcGA1UEAwwQc29tZS5jbHVzdGVyLmNvbTEi
MCAGCSqGSIb3DQEJARYTc2t1em5ldHNAcmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEB
AQUAA0sAMEgCQQC298CIpIW1CQFurW3YScMSLHlyWRY5z3cBnItEOQ+1fn/576fm
NGkzWzlJquOXSU2Smw+kPYakyzdqBDvAG0bjAgMBAAEwDQYJKoZIhvcNAQELBQAD
QQCQFLaY/EJMFEBBYFFB+QHR9wLqYZIm2tJJwH+IsDTb1wLgXNIHpin6uK21vbpn
9au8jPTQdWHakA7JICKK+qc5
-----END CERTIFICATE-----
`),
						KeyData: []byte(`-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,06E0CD2AE5244C0C

gbfOrdO5uf6n5GdnubuCgJHcJLFBAbQc3DNBiy1+ddLxsxTzUMfvYGCmR7MacdAC
lLbAj9jpgxA1pxAihlr+drN3lLLd1kzgRDGu/qMwf9Q+Q+coWffb9XIlS54XORtT
sxUR8HUWxLKqTHevIOihaaeAP9ONLTvk8eCixp+CYKHfXhG2Bp4sdRe1+sm29JeS
LRbeQvHWa3jODnUgh3mBgFPVXzm7labGILy2ME9eb+DIXwOqf0NVSo6QdUJ2OjEG
6Qc+Y/0ssMt5pnmsdohmYyA8zCJep9PcH7FZfg0uvacOdBpDzx56Vx98RDLvGl/L
wOv0lkq/qGws40vFDNgd6aoQc1SZLXodcowP8CdBBBV8DAq6AOQNpeSvP1t92JUq
mXEkQehAoDMnfPLJ1qZB+0f7VqaTjbETJyeW66eio88=
-----END RSA PRIVATE KEY-----
`),
						CAData: []byte(`-----BEGIN CERTIFICATE-----
MIICizCCAjWgAwIBAgIUC/MjPOjQFO8QUdxlsjq0RZpsTOYwDQYJKoZIhvcNAQEL
BQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApXYXNoaW5ndG9uMRAwDgYDVQQH
DAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQgSGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQx
GTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5jb20xIjAgBgkqhkiG9w0BCQEWE3NrdXpu
ZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEwMjM0NzI1WhcNMjAwMjEwMjM0NzI1WjCB
mTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCldhc2hpbmd0b24xEDAOBgNVBAcMB1Nl
YXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQxEjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcG
A1UEAwwQc29tZS5jbHVzdGVyLmNvbTEiMCAGCSqGSIb3DQEJARYTc2t1em5ldHNA
cmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCmLD42NMlT81zyXI+1
eok9DCBO7rXCHqNMfq4doIw8gI1njSWXkn6yOXLyh7lBMJ/bxoElSbmG9Uj/scDR
aIBVAgMBAAGjUzBRMB0GA1UdDgQWBBRu0vBBR5Lk0u5sSlImogZNP8CpNjAfBgNV
HSMEGDAWgBRu0vBBR5Lk0u5sSlImogZNP8CpNjAPBgNVHRMBAf8EBTADAQH/MA0G
CSqGSIb3DQEBCwUAA0EAoBzhZlFCPFdptpf3M8BV0TI2IgmAd3jnZUu3ofnla+up
/XRcQpKJT/uJvvfK+b8BQSUeE2VFMZ0BND2EreZcZQ==
-----END CERTIFICATE-----
`),
					},
				},
			},
			expectedDefault: nil,
			expectedErr:     false,
		},
		{
			name: "explicit build cluster with multiple entries loads fine and sets no default",
			buildCluster: `some-alias:
  endpoint: https://some.cluster.com:443
  clientCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNIakNDQWNnQ0FRRXdEUVlKS29aSWh2Y05BUUVMQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWUQKVlFRSURBcFhZWE5vYVc1bmRHOXVNUkF3RGdZRFZRUUhEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZwpTR0YwTVJJd0VBWURWUVFMREFsUGNHVnVVMmhwWm5ReEdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqCmIyMHhJakFnQmdrcWhraUc5dzBCQ1FFV0UzTnJkWHB1WlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXcKTWpNME9UQXlXaGNOTWpBd01qRXdNak0wT1RBeVdqQ0JtVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTQpDbGRoYzJocGJtZDBiMjR4RURBT0JnTlZCQWNNQjFObFlYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4CkVqQVFCZ05WQkFzTUNVOXdaVzVUYUdsbWRERVpNQmNHQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWkKTUNBR0NTcUdTSWIzRFFFSkFSWVRjMnQxZW01bGRITkFjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQgpBUVVBQTBzQU1FZ0NRUUMyOThDSXBJVzFDUUZ1clczWVNjTVNMSGx5V1JZNXozY0JuSXRFT1ErMWZuLzU3NmZtCk5Ha3pXemxKcXVPWFNVMlNtdytrUFlha3l6ZHFCRHZBRzBiakFnTUJBQUV3RFFZSktvWklodmNOQVFFTEJRQUQKUVFDUUZMYVkvRUpNRkVCQllGRkIrUUhSOXdMcVlaSW0ydEpKd0grSXNEVGIxd0xnWE5JSHBpbjZ1SzIxdmJwbgo5YXU4alBUUWRXSGFrQTdKSUNLSytxYzUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
  clientKey: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpQcm9jLVR5cGU6IDQsRU5DUllQVEVECkRFSy1JbmZvOiBERVMtRURFMy1DQkMsMDZFMENEMkFFNTI0NEMwQwoKZ2JmT3JkTzV1ZjZuNUdkbnVidUNnSkhjSkxGQkFiUWMzRE5CaXkxK2RkTHhzeFR6VU1mdllHQ21SN01hY2RBQwpsTGJBajlqcGd4QTFweEFpaGxyK2RyTjNsTExkMWt6Z1JER3UvcU13ZjlRK1ErY29XZmZiOVhJbFM1NFhPUnRUCnN4VVI4SFVXeExLcVRIZXZJT2loYWFlQVA5T05MVHZrOGVDaXhwK0NZS0hmWGhHMkJwNHNkUmUxK3NtMjlKZVMKTFJiZVF2SFdhM2pPRG5VZ2gzbUJnRlBWWHptN2xhYkdJTHkyTUU5ZWIrRElYd09xZjBOVlNvNlFkVUoyT2pFRwo2UWMrWS8wc3NNdDVwbm1zZG9obVl5QTh6Q0plcDlQY0g3RlpmZzB1dmFjT2RCcER6eDU2Vng5OFJETHZHbC9MCndPdjBsa3EvcUd3czQwdkZETmdkNmFvUWMxU1pMWG9kY293UDhDZEJCQlY4REFxNkFPUU5wZVN2UDF0OTJKVXEKbVhFa1FlaEFvRE1uZlBMSjFxWkIrMGY3VnFhVGpiRVRKeWVXNjZlaW84OD0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K
  clusterCaCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNpekNDQWpXZ0F3SUJBZ0lVQy9NalBPalFGTzhRVWR4bHNqcTBSWnBzVE9Zd0RRWUpLb1pJaHZjTkFRRUwKQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJREFwWFlYTm9hVzVuZEc5dU1SQXdEZ1lEVlFRSApEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZ1NHRjBNUkl3RUFZRFZRUUxEQWxQY0dWdVUyaHBablF4CkdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqYjIweElqQWdCZ2txaGtpRzl3MEJDUUVXRTNOcmRYcHUKWlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXdNak0wTnpJMVdoY05NakF3TWpFd01qTTBOekkxV2pDQgptVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTUNsZGhjMmhwYm1kMGIyNHhFREFPQmdOVkJBY01CMU5sCllYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4RWpBUUJnTlZCQXNNQ1U5d1pXNVRhR2xtZERFWk1CY0cKQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWlNQ0FHQ1NxR1NJYjNEUUVKQVJZVGMydDFlbTVsZEhOQQpjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQkFRVUFBMHNBTUVnQ1FRQ21MRDQyTk1sVDgxenlYSSsxCmVvazlEQ0JPN3JYQ0hxTk1mcTRkb0l3OGdJMW5qU1dYa242eU9YTHloN2xCTUovYnhvRWxTYm1HOVVqL3NjRFIKYUlCVkFnTUJBQUdqVXpCUk1CMEdBMVVkRGdRV0JCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFmQmdOVgpIU01FR0RBV2dCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQTBHCkNTcUdTSWIzRFFFQkN3VUFBMEVBb0J6aFpsRkNQRmRwdHBmM004QlYwVEkySWdtQWQzam5aVXUzb2ZubGErdXAKL1hSY1FwS0pUL3VKdnZmSytiOEJRU1VlRTJWRk1aMEJORDJFcmVaY1pRPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
other-alias-same-cluster:
  endpoint: https://some.cluster.com:443
  clientCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNIakNDQWNnQ0FRRXdEUVlKS29aSWh2Y05BUUVMQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWUQKVlFRSURBcFhZWE5vYVc1bmRHOXVNUkF3RGdZRFZRUUhEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZwpTR0YwTVJJd0VBWURWUVFMREFsUGNHVnVVMmhwWm5ReEdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqCmIyMHhJakFnQmdrcWhraUc5dzBCQ1FFV0UzTnJkWHB1WlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXcKTWpNME9UQXlXaGNOTWpBd01qRXdNak0wT1RBeVdqQ0JtVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTQpDbGRoYzJocGJtZDBiMjR4RURBT0JnTlZCQWNNQjFObFlYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4CkVqQVFCZ05WQkFzTUNVOXdaVzVUYUdsbWRERVpNQmNHQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWkKTUNBR0NTcUdTSWIzRFFFSkFSWVRjMnQxZW01bGRITkFjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQgpBUVVBQTBzQU1FZ0NRUUMyOThDSXBJVzFDUUZ1clczWVNjTVNMSGx5V1JZNXozY0JuSXRFT1ErMWZuLzU3NmZtCk5Ha3pXemxKcXVPWFNVMlNtdytrUFlha3l6ZHFCRHZBRzBiakFnTUJBQUV3RFFZSktvWklodmNOQVFFTEJRQUQKUVFDUUZMYVkvRUpNRkVCQllGRkIrUUhSOXdMcVlaSW0ydEpKd0grSXNEVGIxd0xnWE5JSHBpbjZ1SzIxdmJwbgo5YXU4alBUUWRXSGFrQTdKSUNLSytxYzUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
  clientKey: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpQcm9jLVR5cGU6IDQsRU5DUllQVEVECkRFSy1JbmZvOiBERVMtRURFMy1DQkMsMDZFMENEMkFFNTI0NEMwQwoKZ2JmT3JkTzV1ZjZuNUdkbnVidUNnSkhjSkxGQkFiUWMzRE5CaXkxK2RkTHhzeFR6VU1mdllHQ21SN01hY2RBQwpsTGJBajlqcGd4QTFweEFpaGxyK2RyTjNsTExkMWt6Z1JER3UvcU13ZjlRK1ErY29XZmZiOVhJbFM1NFhPUnRUCnN4VVI4SFVXeExLcVRIZXZJT2loYWFlQVA5T05MVHZrOGVDaXhwK0NZS0hmWGhHMkJwNHNkUmUxK3NtMjlKZVMKTFJiZVF2SFdhM2pPRG5VZ2gzbUJnRlBWWHptN2xhYkdJTHkyTUU5ZWIrRElYd09xZjBOVlNvNlFkVUoyT2pFRwo2UWMrWS8wc3NNdDVwbm1zZG9obVl5QTh6Q0plcDlQY0g3RlpmZzB1dmFjT2RCcER6eDU2Vng5OFJETHZHbC9MCndPdjBsa3EvcUd3czQwdkZETmdkNmFvUWMxU1pMWG9kY293UDhDZEJCQlY4REFxNkFPUU5wZVN2UDF0OTJKVXEKbVhFa1FlaEFvRE1uZlBMSjFxWkIrMGY3VnFhVGpiRVRKeWVXNjZlaW84OD0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K
  clusterCaCertificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNpekNDQWpXZ0F3SUJBZ0lVQy9NalBPalFGTzhRVWR4bHNqcTBSWnBzVE9Zd0RRWUpLb1pJaHZjTkFRRUwKQlFBd2daa3hDekFKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJREFwWFlYTm9hVzVuZEc5dU1SQXdEZ1lEVlFRSApEQWRUWldGMGRHeGxNUkF3RGdZRFZRUUtEQWRTWldRZ1NHRjBNUkl3RUFZRFZRUUxEQWxQY0dWdVUyaHBablF4CkdUQVhCZ05WQkFNTUVITnZiV1V1WTJ4MWMzUmxjaTVqYjIweElqQWdCZ2txaGtpRzl3MEJDUUVXRTNOcmRYcHUKWlhSelFISmxaR2hoZEM1amIyMHdIaGNOTVRrd01qRXdNak0wTnpJMVdoY05NakF3TWpFd01qTTBOekkxV2pDQgptVEVMTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnTUNsZGhjMmhwYm1kMGIyNHhFREFPQmdOVkJBY01CMU5sCllYUjBiR1V4RURBT0JnTlZCQW9NQjFKbFpDQklZWFF4RWpBUUJnTlZCQXNNQ1U5d1pXNVRhR2xtZERFWk1CY0cKQTFVRUF3d1FjMjl0WlM1amJIVnpkR1Z5TG1OdmJURWlNQ0FHQ1NxR1NJYjNEUUVKQVJZVGMydDFlbTVsZEhOQQpjbVZrYUdGMExtTnZiVEJjTUEwR0NTcUdTSWIzRFFFQkFRVUFBMHNBTUVnQ1FRQ21MRDQyTk1sVDgxenlYSSsxCmVvazlEQ0JPN3JYQ0hxTk1mcTRkb0l3OGdJMW5qU1dYa242eU9YTHloN2xCTUovYnhvRWxTYm1HOVVqL3NjRFIKYUlCVkFnTUJBQUdqVXpCUk1CMEdBMVVkRGdRV0JCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFmQmdOVgpIU01FR0RBV2dCUnUwdkJCUjVMazB1NXNTbEltb2daTlA4Q3BOakFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQTBHCkNTcUdTSWIzRFFFQkN3VUFBMEVBb0J6aFpsRkNQRmRwdHBmM004QlYwVEkySWdtQWQzam5aVXUzb2ZubGErdXAKL1hSY1FwS0pUL3VKdnZmSytiOEJRU1VlRTJWRk1aMEJORDJFcmVaY1pRPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
`,
			expected: map[string]rest.Config{
				"some-alias": {
					Host: "https://some.cluster.com:443",
					TLSClientConfig: rest.TLSClientConfig{
						CertData: []byte(`-----BEGIN CERTIFICATE-----
MIICHjCCAcgCAQEwDQYJKoZIhvcNAQELBQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYD
VQQIDApXYXNoaW5ndG9uMRAwDgYDVQQHDAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQg
SGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQxGTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5j
b20xIjAgBgkqhkiG9w0BCQEWE3NrdXpuZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEw
MjM0OTAyWhcNMjAwMjEwMjM0OTAyWjCBmTELMAkGA1UEBhMCVVMxEzARBgNVBAgM
Cldhc2hpbmd0b24xEDAOBgNVBAcMB1NlYXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQx
EjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcGA1UEAwwQc29tZS5jbHVzdGVyLmNvbTEi
MCAGCSqGSIb3DQEJARYTc2t1em5ldHNAcmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEB
AQUAA0sAMEgCQQC298CIpIW1CQFurW3YScMSLHlyWRY5z3cBnItEOQ+1fn/576fm
NGkzWzlJquOXSU2Smw+kPYakyzdqBDvAG0bjAgMBAAEwDQYJKoZIhvcNAQELBQAD
QQCQFLaY/EJMFEBBYFFB+QHR9wLqYZIm2tJJwH+IsDTb1wLgXNIHpin6uK21vbpn
9au8jPTQdWHakA7JICKK+qc5
-----END CERTIFICATE-----
`),
						KeyData: []byte(`-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,06E0CD2AE5244C0C

gbfOrdO5uf6n5GdnubuCgJHcJLFBAbQc3DNBiy1+ddLxsxTzUMfvYGCmR7MacdAC
lLbAj9jpgxA1pxAihlr+drN3lLLd1kzgRDGu/qMwf9Q+Q+coWffb9XIlS54XORtT
sxUR8HUWxLKqTHevIOihaaeAP9ONLTvk8eCixp+CYKHfXhG2Bp4sdRe1+sm29JeS
LRbeQvHWa3jODnUgh3mBgFPVXzm7labGILy2ME9eb+DIXwOqf0NVSo6QdUJ2OjEG
6Qc+Y/0ssMt5pnmsdohmYyA8zCJep9PcH7FZfg0uvacOdBpDzx56Vx98RDLvGl/L
wOv0lkq/qGws40vFDNgd6aoQc1SZLXodcowP8CdBBBV8DAq6AOQNpeSvP1t92JUq
mXEkQehAoDMnfPLJ1qZB+0f7VqaTjbETJyeW66eio88=
-----END RSA PRIVATE KEY-----
`),
						CAData: []byte(`-----BEGIN CERTIFICATE-----
MIICizCCAjWgAwIBAgIUC/MjPOjQFO8QUdxlsjq0RZpsTOYwDQYJKoZIhvcNAQEL
BQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApXYXNoaW5ndG9uMRAwDgYDVQQH
DAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQgSGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQx
GTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5jb20xIjAgBgkqhkiG9w0BCQEWE3NrdXpu
ZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEwMjM0NzI1WhcNMjAwMjEwMjM0NzI1WjCB
mTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCldhc2hpbmd0b24xEDAOBgNVBAcMB1Nl
YXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQxEjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcG
A1UEAwwQc29tZS5jbHVzdGVyLmNvbTEiMCAGCSqGSIb3DQEJARYTc2t1em5ldHNA
cmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCmLD42NMlT81zyXI+1
eok9DCBO7rXCHqNMfq4doIw8gI1njSWXkn6yOXLyh7lBMJ/bxoElSbmG9Uj/scDR
aIBVAgMBAAGjUzBRMB0GA1UdDgQWBBRu0vBBR5Lk0u5sSlImogZNP8CpNjAfBgNV
HSMEGDAWgBRu0vBBR5Lk0u5sSlImogZNP8CpNjAPBgNVHRMBAf8EBTADAQH/MA0G
CSqGSIb3DQEBCwUAA0EAoBzhZlFCPFdptpf3M8BV0TI2IgmAd3jnZUu3ofnla+up
/XRcQpKJT/uJvvfK+b8BQSUeE2VFMZ0BND2EreZcZQ==
-----END CERTIFICATE-----
`),
					},
				},
				"other-alias-same-cluster": {
					Host: "https://some.cluster.com:443",
					TLSClientConfig: rest.TLSClientConfig{
						CertData: []byte(`-----BEGIN CERTIFICATE-----
MIICHjCCAcgCAQEwDQYJKoZIhvcNAQELBQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYD
VQQIDApXYXNoaW5ndG9uMRAwDgYDVQQHDAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQg
SGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQxGTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5j
b20xIjAgBgkqhkiG9w0BCQEWE3NrdXpuZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEw
MjM0OTAyWhcNMjAwMjEwMjM0OTAyWjCBmTELMAkGA1UEBhMCVVMxEzARBgNVBAgM
Cldhc2hpbmd0b24xEDAOBgNVBAcMB1NlYXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQx
EjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcGA1UEAwwQc29tZS5jbHVzdGVyLmNvbTEi
MCAGCSqGSIb3DQEJARYTc2t1em5ldHNAcmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEB
AQUAA0sAMEgCQQC298CIpIW1CQFurW3YScMSLHlyWRY5z3cBnItEOQ+1fn/576fm
NGkzWzlJquOXSU2Smw+kPYakyzdqBDvAG0bjAgMBAAEwDQYJKoZIhvcNAQELBQAD
QQCQFLaY/EJMFEBBYFFB+QHR9wLqYZIm2tJJwH+IsDTb1wLgXNIHpin6uK21vbpn
9au8jPTQdWHakA7JICKK+qc5
-----END CERTIFICATE-----
`),
						KeyData: []byte(`-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,06E0CD2AE5244C0C

gbfOrdO5uf6n5GdnubuCgJHcJLFBAbQc3DNBiy1+ddLxsxTzUMfvYGCmR7MacdAC
lLbAj9jpgxA1pxAihlr+drN3lLLd1kzgRDGu/qMwf9Q+Q+coWffb9XIlS54XORtT
sxUR8HUWxLKqTHevIOihaaeAP9ONLTvk8eCixp+CYKHfXhG2Bp4sdRe1+sm29JeS
LRbeQvHWa3jODnUgh3mBgFPVXzm7labGILy2ME9eb+DIXwOqf0NVSo6QdUJ2OjEG
6Qc+Y/0ssMt5pnmsdohmYyA8zCJep9PcH7FZfg0uvacOdBpDzx56Vx98RDLvGl/L
wOv0lkq/qGws40vFDNgd6aoQc1SZLXodcowP8CdBBBV8DAq6AOQNpeSvP1t92JUq
mXEkQehAoDMnfPLJ1qZB+0f7VqaTjbETJyeW66eio88=
-----END RSA PRIVATE KEY-----
`),
						CAData: []byte(`-----BEGIN CERTIFICATE-----
MIICizCCAjWgAwIBAgIUC/MjPOjQFO8QUdxlsjq0RZpsTOYwDQYJKoZIhvcNAQEL
BQAwgZkxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApXYXNoaW5ndG9uMRAwDgYDVQQH
DAdTZWF0dGxlMRAwDgYDVQQKDAdSZWQgSGF0MRIwEAYDVQQLDAlPcGVuU2hpZnQx
GTAXBgNVBAMMEHNvbWUuY2x1c3Rlci5jb20xIjAgBgkqhkiG9w0BCQEWE3NrdXpu
ZXRzQHJlZGhhdC5jb20wHhcNMTkwMjEwMjM0NzI1WhcNMjAwMjEwMjM0NzI1WjCB
mTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCldhc2hpbmd0b24xEDAOBgNVBAcMB1Nl
YXR0bGUxEDAOBgNVBAoMB1JlZCBIYXQxEjAQBgNVBAsMCU9wZW5TaGlmdDEZMBcG
A1UEAwwQc29tZS5jbHVzdGVyLmNvbTEiMCAGCSqGSIb3DQEJARYTc2t1em5ldHNA
cmVkaGF0LmNvbTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCmLD42NMlT81zyXI+1
eok9DCBO7rXCHqNMfq4doIw8gI1njSWXkn6yOXLyh7lBMJ/bxoElSbmG9Uj/scDR
aIBVAgMBAAGjUzBRMB0GA1UdDgQWBBRu0vBBR5Lk0u5sSlImogZNP8CpNjAfBgNV
HSMEGDAWgBRu0vBBR5Lk0u5sSlImogZNP8CpNjAPBgNVHRMBAf8EBTADAQH/MA0G
CSqGSIb3DQEBCwUAA0EAoBzhZlFCPFdptpf3M8BV0TI2IgmAd3jnZUu3ofnla+up
/XRcQpKJT/uJvvfK+b8BQSUeE2VFMZ0BND2EreZcZQ==
-----END CERTIFICATE-----
`),
					},
				},
			},
			expectedDefault: nil,
			expectedErr:     false,
		},
		{
			name: "invalid build cluster does not load",
			buildCluster: `some-alias:
  endpoint: {}
`,
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buildCluster, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatalf("%s: could not create build cluster file: %v", testCase.name, err)
			}
			defer func() {
				if err := os.Remove(buildCluster.Name()); err != nil {
					t.Fatalf("%s: failed to clean up temp file: %v", testCase.name, err)
				}
			}()
			if _, err := buildCluster.WriteString(testCase.buildCluster); err != nil {
				t.Fatalf("%s: could not populate build cluster file: %v", testCase.name, err)
			}

			configurations, defaultContext, err := buildClusterConfigLoader(buildCluster.Name())()
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !reflect.DeepEqual(configurations, testCase.expected) {
				t.Errorf("%s: got incorrect cluster configurations: %s", testCase.name, diff.ObjectReflectDiff(configurations, testCase.expected))
			}
			if !reflect.DeepEqual(defaultContext, testCase.expectedDefault) {
				t.Errorf("%s: got incorrect default context: %s", testCase.name, diff.ObjectReflectDiff(defaultContext, testCase.expectedDefault))
			}
		})
	}
}

func TestAggregateClusterConfigLoader(t *testing.T) {
	var testCases = []struct {
		name    string
		loaders []clusterConfigLoader

		expected        map[string]rest.Config
		expectedDefault *string
		expectedErr     bool
	}{
		{
			name: "mutually exclusive contexts with one default loads fine",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, nil, nil
				},
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"second": {Host: "second.com"}}, strPointer("second"), nil
				},
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"third": {Host: "third.com"}}, nil, nil
				},
			},
			expected: map[string]rest.Config{
				"first":  {Host: "first.com"},
				"second": {Host: "second.com"},
				"third":  {Host: "third.com"},
			},
			expectedDefault: strPointer("second"),
			expectedErr:     false,
		},
		{
			name: "contexts with no default uses in-cluster context as default",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{*inClusterContext(): {Host: "in-cluster.com"}}, nil, nil
				},
			},
			expected: map[string]rest.Config{
				*inClusterContext(): {Host: "in-cluster.com"},
			},
			expectedDefault: inClusterContext(),
			expectedErr:     false,
		},
		{
			name: "contexts with no default and no in-cluster context errors",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, nil, nil
				},
			},
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
		{
			name: "mutually exclusive contexts with two defaults errors",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, strPointer("first"), nil
				},
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"second": {Host: "second.com"}}, strPointer("second"), nil
				},
			},
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
		{
			name: "overlapping contexts errors",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, nil, nil
				},
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, nil, nil
				},
			},
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
		{
			name: "error loading one loader errors",
			loaders: []clusterConfigLoader{
				func() (map[string]rest.Config, *string, error) {
					return map[string]rest.Config{"first": {Host: "first.com"}}, nil, nil
				},
				func() (map[string]rest.Config, *string, error) {
					return nil, nil, errors.New("error loading")
				},
			},
			expected:        nil,
			expectedDefault: nil,
			expectedErr:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configurations, defaultContext, err := aggregateClusterConfigLoader(testCase.loaders...)()
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !reflect.DeepEqual(configurations, testCase.expected) {
				t.Errorf("%s: got incorrect cluster configurations: %s", testCase.name, diff.ObjectReflectDiff(configurations, testCase.expected))
			}
			if !reflect.DeepEqual(defaultContext, testCase.expectedDefault) {
				t.Errorf("%s: got incorrect default context: %s", testCase.name, diff.ObjectReflectDiff(defaultContext, testCase.expectedDefault))
			}
		})
	}
}
