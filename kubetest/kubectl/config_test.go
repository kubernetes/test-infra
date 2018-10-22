/*
Copyright 2018 The Kubernetes Authors.

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

package kubectl

import (
	"testing"
)

func TestParseKubeconfig(t *testing.T) {
	output := `
{
    "kind": "Config",
    "apiVersion": "v1",
    "preferences": {},
    "clusters": [
        {
            "name": "gke_testproject_us-central1-a_testcluster",
            "cluster": {
                "server": "https://10.10.10.10",
                "certificate-authority-data": "LS0t...removed..."
            }
        }
    ],
    "users": [
        {
            "name": "gke_testproject_us-central1-a_testcluster",
            "user": {
                "auth-provider": {
                    "name": "gcp",
                    "config": {
                        "access-token": "ya29.foo",
                        "cmd-args": "config config-helper --format=json",
                        "cmd-path": "/usr/lib/google-cloud-sdk/bin/gcloud",
                        "expiry": "2018-10-03T18:42:11Z",
                        "expiry-key": "{.credential.token_expiry}",
                        "token-key": "{.credential.access_token}"
                    }
                }
            }
        }
    ],
    "contexts": [
        {
            "name": "gke_testproject_us-central1-a_testcluster",
            "context": {
                "cluster": "gke_testproject_us-central1-a_testcluster",
                "user": "gke_testproject_us-central1-a_testcluster",
                "namespace": "default"
            }
        }
    ],
    "current-context": "gke_testproject_us-central1-a_testcluster"
}
`

	config, err := parseConfig([]byte(output))
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	t.Logf("parsed as %+v", config)

	actual, found := config.CurrentServer()
	if !found {
		t.Fatalf("server was not found")
	}

	expected := "https://10.10.10.10"
	if actual != expected {
		t.Fatalf("server was not as expected.  actual=%q, expected=%q", actual, expected)
	}
}
