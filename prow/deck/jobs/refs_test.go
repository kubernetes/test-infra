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

package jobs

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

func TestGetRefData(t *testing.T) {
	cases := []struct {
		name string
		job  kube.ProwJob
		exp  RefData
	}{
		{
			name: "github presubmit (1 PR)",
			job: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Refs: &kube.Refs{
						Org:     "kubernetes",
						Repo:    "prow",
						BaseRef: "master",
						BaseSHA: "12345",
						Pulls: []kube.Pull{
							{
								Number: 23,
								Author: "ibzib",
								SHA:    "99999",
							},
						},
					},
				},
			},
			exp: RefData{
				Repo:           "kubernetes/prow",
				Refs:           "master:12345,23:99999",
				BaseRef:        "master",
				BaseSHA:        "12345",
				PullSHA:        "99999",
				Number:         23,
				Author:         "ibzib",
				RepoLink:       "https://github.com/kubernetes/prow",
				PullLink:       "https://github.com/kubernetes/prow/pull/23",
				PullCommitLink: "https://github.com/kubernetes/prow/pull/23/commits/99999",
				PushCommitLink: "https://github.com/kubernetes/prow/commit/12345",
				AuthorLink:     "https://github.com/ibzib",
				PRRefs:         []int{23},
				PRRefLinks:     []string{"https://github.com/kubernetes/prow/pull/23"},
			},
		},
		{
			name: "github batch job",
			job: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Refs: &kube.Refs{
						Org:     "kubernetes",
						Repo:    "prow",
						BaseRef: "master",
						BaseSHA: "12345",
						Pulls: []kube.Pull{
							{
								Number: 23,
								Author: "ibzib",
								SHA:    "99999",
							},
							{
								Number: 24,
								Author: "k8s-ci-robot",
								SHA:    "nomorehumans",
							},
						},
					},
				},
			},
			exp: RefData{
				Repo:           "kubernetes/prow",
				Refs:           "master:12345,23:99999,24:nomorehumans",
				BaseRef:        "master",
				BaseSHA:        "12345",
				RepoLink:       "https://github.com/kubernetes/prow",
				PushCommitLink: "https://github.com/kubernetes/prow/commit/12345",
				PRRefs:         []int{23, 24},
				PRRefLinks: []string{
					"https://github.com/kubernetes/prow/pull/23",
					"https://github.com/kubernetes/prow/pull/24",
				},
			},
		},
		{
			name: "gerrit presubmit",
			job: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						client.GerritInstance: "https://fhqwhgads-review.example.com",
						client.GerritID:       "trogdor%2Fdragonman~I12345",
					},
					Labels: map[string]string{
						client.GerritRevision: "12345",
					},
				},
				Spec: kube.ProwJobSpec{
					Refs: &kube.Refs{
						Org:     "fhqwhgads-review.example.com",
						Repo:    "trogdor/dragonman",
						BaseRef: "master",
						BaseSHA: "12345",
						Pulls: []kube.Pull{
							{
								Number: 23,
								Author: "strongbad@example.com",
								SHA:    "99999",
							},
						},
					},
				},
			},
			exp: RefData{
				Repo:           "trogdor/dragonman",
				Refs:           "master:12345,23:99999",
				BaseRef:        "master",
				BaseSHA:        "12345",
				PullSHA:        "99999",
				Number:         23,
				Author:         "strongbad@example.com",
				RepoLink:       "https://fhqwhgads.example.com/trogdor/dragonman",
				PullLink:       "https://fhqwhgads-review.example.com/c/trogdor/dragonman/+/23",
				PullCommitLink: "https://fhqwhgads.example.com/trogdor/dragonman/+/99999",
				PushCommitLink: "https://fhqwhgads.example.com/trogdor/dragonman/+/12345",
				AuthorLink:     "mailto:strongbad@example.com",
			},
		},
	}
	for _, tc := range cases {
		refData := getRefData(tc.job)
		if !reflect.DeepEqual(refData, tc.exp) {
			t.Errorf("%s\nexpected: %v\n"+
				"got:      %v", tc.name, tc.exp, refData)
		}
	}
}
