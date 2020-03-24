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

// Resultstore converts --build=gs://prefix/JOB/NUMBER from prow's pod-utils to a ResultStore invocation suite, which it optionally will --upload=gcp-project.
package main

import (
	"context"
	"reflect"
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func TestInsertLink(t *testing.T) {
	const viewURL = "http://result/store/url"
	cases := []struct {
		name     string
		input    metadata.Metadata
		expected metadata.Metadata
		changed  bool
		err      bool
	}{
		{
			name:  "adds metadata when missing",
			input: metadata.Metadata{},
			expected: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey: viewURL,
					},
				},
				resultstoreKey: viewURL,
			},
			changed: true,
		},
		{
			name: "override resultstore metadata",
			input: metadata.Metadata{
				resultstoreKey: 234342,
			},
			expected: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey: viewURL,
					},
				},
				resultstoreKey: viewURL,
			},
			changed: true,
		},
		{
			name: "do not overwrite links key of wrong type",
			input: metadata.Metadata{
				linksKey: "unexpected type",
			},
			err: true,
		},
		{
			name: "do not overwrite resultstore link of wrong type",
			input: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: "unexpected type",
				},
			},
			err: true,
		},
		{
			name: "do not overwrite resultstore url link of wrong type",
			input: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey: 1234,
					},
				},
			},
			err: true,
		},
		{
			name: "add to existing links",
			input: metadata.Metadata{
				linksKey: metadata.Metadata{
					"random key": 1234,
				},
			},
			expected: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey: viewURL,
					},
					"random key": 1234,
				},
				resultstoreKey: viewURL,
			},
			changed: true,
		},
		{
			name: "update existing resultstore url",
			input: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey:        "random url",
						"extra stuff": "gets preserved",
					},
				},
			},
			expected: metadata.Metadata{
				linksKey: metadata.Metadata{
					resultstoreKey: metadata.Metadata{
						urlKey:        viewURL,
						"extra stuff": "gets preserved",
					},
				},
				resultstoreKey: viewURL,
			},
			changed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var start gcs.Started
			start.Metadata = tc.input
			changed, err := insertLink(&start, viewURL)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to received an error")
			case changed != tc.changed:
				t.Errorf("changed %t != expected %t", changed, tc.changed)
			case !reflect.DeepEqual(tc.expected, tc.input):
				t.Errorf("metadata %#v != expected %#v", tc.input, tc.expected)
			}
		})
	}
}

func TestFilterBuckets(t *testing.T) {
	cases := []struct {
		name     string
		groups   []configpb.TestGroup
		paths    []string
		expected []configpb.TestGroup
		err      bool
	}{
		{
			name: "bad groups error",
			groups: []configpb.TestGroup{
				{
					Name:      "foo",
					GcsPrefix: "oh\n/yeah",
				},
				{
					Name: "bar",
				},
			},
			paths: []string{"gs://ignored"},
			err:   true,
		},
		{
			name: "filter buckets",
			groups: []configpb.TestGroup{
				{
					Name:      "yes",
					GcsPrefix: "included/bucket",
				},
				{
					Name:      "no",
					GcsPrefix: "excluded/bucket",
				},
			},
			paths: []string{"gs://included"},
			expected: []configpb.TestGroup{
				{
					Name:      "yes",
					GcsPrefix: "included/bucket",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			checkBuckets, err := bucketListChecker(tc.paths...)
			if err != nil {
				t.Fatalf("create checker: %v", err)
			}
			actual, err := filterBuckets(context.Background(), checkBuckets, tc.groups...)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to received expected error")
			case !reflect.DeepEqual(tc.expected, actual):
				t.Errorf("filterBuckets() got %v, want %v", actual, tc.expected)
			}
		})
	}
}
