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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGangwayConfig(t *testing.T) {
	testCases := []struct {
		name          string
		gangwayConfig string
		expectError   bool
	}{
		{
			name: "basic valid case",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
      endpoint_api_consumer_number: "123"
    allowed_jobs_filters:
    - tenant_id: "well-behaved-tenant-for-gangway"
`,
			expectError: false,
		},
		{
			name: "missing allowed_jobs_filters",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
      endpoint_api_consumer_number: "123"
`,
			expectError: true,
		},
		{
			name: "tenant_id is an empty string",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
      endpoint_api_consumer_number: "123"
    - tenant_id: ""
`,
			expectError: true,
		},
		{
			name: "missing cloud vendor",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - allowed_jobs_filters:
    - tenant_id: "well-behaved-tenant-for-gangway"
`,
			expectError: true,
		},
		{
			name: "missing project type",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_number: "123"
    allowed_jobs_filters:
    - tenant_id: "well-behaved-tenant-for-gangway"
`,
			expectError: true,
		},
		{
			name: "missing project number",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
    allowed_jobs_filters:
    - tenant_id: "well-behaved-tenant-for-gangway"
`,
			expectError: true,
		},
		{
			name: "multiple clients with the same endpoint_api_consumer_number",
			gangwayConfig: `
gangway:
  allowed_api_clients:
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
      endpoint_api_consumer_number: "123"
    allowed_jobs_filters:
    - tenant_id: "well-behaved-tenant-for-gangway"
  - gcp:
      endpoint_api_consumer_type: "PROJECT"
      endpoint_api_consumer_number: "123"
    allowed_jobs_filters:
    - tenant_id: "another-client"
`,
			expectError: true,
		},
	}
	for _, tc := range testCases {
		// save the config
		gangwayConfigDir := t.TempDir()

		gangwayConfig := filepath.Join(gangwayConfigDir, "config.yaml")
		if err := os.WriteFile(gangwayConfig, []byte(tc.gangwayConfig), 0666); err != nil {
			t.Fatalf("fail to write gangway config: %v", err)
		}

		_, err := Load(gangwayConfig, "", nil, "")

		if (err != nil) != tc.expectError {
			t.Fatalf("tc %s: expected error: %v, got: %v, error: %v", tc.name, tc.expectError, (err != nil), err)
		}
	}
}
