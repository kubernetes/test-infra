/*
Copyright 2022 The Kubernetes Authors.

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

package podinfo

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses/fake"
)

func TestBody(t *testing.T) {
	tests := []struct {
		name      string
		artifacts []api.Artifact
		tmpl      string
		ownConfig *ownConfig
		want      string
	}{
		{
			name: "base",
			artifacts: []api.Artifact{
				&fake.Artifact{
					Path: "podinfo.json",
					Content: []byte(`{
  "pod": {
      "metadata": {
        "name": "abc-123"
      }
  }
}`),
				},
				&fake.Artifact{
					Path: "prowjob.json",
					Content: []byte(`{
"spec": {
  "cluster": "bar"
}
}`),
				},
			},
			ownConfig: &ownConfig{
				RunnerConfigs: map[string]RunnerConfig{
					"bar": {
						PodLinkTemplate: "http://somewhere/pod/{{ .Name }}",
					},
				},
			},
		},
		{
			name: "no-prowjob-json",
			artifacts: []api.Artifact{
				&fake.Artifact{
					Path: "podinfo.json",
					Content: []byte(`{
  "pod": {
      "metadata": {
        "name": "abc-123"
      }
  }
}`),
				},
			},
			ownConfig: &ownConfig{
				RunnerConfigs: map[string]RunnerConfig{
					"bar": {
						PodLinkTemplate: "http://somewhere/pod/{{ .Name }}",
					},
				},
			},
		},
		{
			name: "no-cluster-info",
			artifacts: []api.Artifact{
				&fake.Artifact{
					Path: "podinfo.json",
					Content: []byte(`{
  "pod": {
      "metadata": {
        "name": "abc-123"
      }
  }
}`),
				},
				&fake.Artifact{
					Path: "prowjob.json",
					Content: []byte(`{
"spec": {
  "cluster": "bar"
}
}`),
				},
			},
			ownConfig: nil,
		},
	}

	logHook := test.NewGlobal()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logHook.Reset()
			wantFile := path.Join("testdata", "test_"+tc.name+".html")
			wantBytes, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("Failed reading output file %s: %v", wantFile, err)
			}
			var oc []byte
			if tc.ownConfig != nil {
				oc, err = json.Marshal(tc.ownConfig)
				if err != nil {
					t.Fatal(err)
				}
			}
			got, want := Lens{}.Body(tc.artifacts, ".", "", json.RawMessage(oc), config.Spyglass{}), string(wantBytes)
			if got != want {
				t.Errorf("Output mismatch\nwant: %s\n got: %s", want, got)
			}
			if entries := logHook.AllEntries(); len(entries) > 0 {
				var logs []string
				for _, entry := range entries {
					log, _ := entry.String()
					logs = append(logs, log)
				}
				t.Errorf("Unexpected log messages: %v", logs)
			}
		})
	}
}
