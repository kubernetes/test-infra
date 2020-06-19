package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	// "reflect"
	"testing"
)

type clientMock struct { // mock client

}

func TestParse(t *testing.T) {
	var tests = []struct {
		input []byte
		want  SyncPairCollection
	}{
		{
			input: []byte(`specs:
- source: 
    kubernetes:
      namespace: ns1
      secret: secret1
  destination:
    secretManager:
      project: k8s-jkns-gke-soak
      secret: test-secret
- source: 
    secretManager:
      project: k8s-jkns-gke-soak
      secret: docker-secret
  destination:
    kubernetes:
      namespace: default
      secret: docker-secret`),
			want: SyncPairCollection{
				Pairs: []SyncPair{
					{
						Source: Target{
							Kubernetes: &KubernetesSecret{
								Namespace: "ns1",
								Secret:    "secret1",
							},
						},
						Destination: Target{
							SecretManager: &SecretManagerSecret{
								Project: "k8s-jkns-gke-soak",
								Secret:  "test-secret",
							},
						},
					},
					{
						Source: Target{
							SecretManager: &SecretManagerSecret{
								Project: "k8s-jkns-gke-soak",
								Secret:  "docker-secret",
							},
						},
						Destination: Target{
							Kubernetes: &KubernetesSecret{
								Namespace: "default",
								Secret:    "secret1",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.input)
		t.Run(testname, func(t *testing.T) {
			syncConfig := SecretSyncConfig{}
			err := yaml.Unmarshal(tt.input, &syncConfig)
			if err != nil {
				t.Errorf("%s", err)
			}
			collection := syncConfig.Parse()
			for i, pair := range collection.Pairs {
				var target Target
				target = pair.Source
				if k8s, gsm := target.Kubernetes, target.SecretManager; k8s != nil {
					if fmt.Sprintf("%s", *k8s) != fmt.Sprintf("%s", *tt.want.Pairs[i].Source.Kubernetes) {
						t.Errorf("Pair %d has source got %s, want %s", i, *k8s, (*tt.want.Pairs[i].Source.Kubernetes))
					}
				} else {
					if fmt.Sprintf("%s", *gsm) != fmt.Sprintf("%s", *tt.want.Pairs[i].Source.SecretManager) {
						t.Errorf("Pair %d has source got %s, want %s", i, *gsm, (*tt.want.Pairs[i].Source.SecretManager))
					}
				}
			}
		})
	}
}

// fmt.Println(reflect.DeepEqual(m1, m2))
