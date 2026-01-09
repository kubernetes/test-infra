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

package summarize

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	testCases := []struct {
		name     string
		argument string
		want     string
	}{
		{"Hex strings, letters, version number", "0x1234 a 123.13.45.43 b 2e24e003-9ffd-4e78-852c-9dcb6cbef493-123", "UNIQ1 a UNIQ2 b UNIQ3"},
		{"Date and time", "Mon, 12 January 2017 11:34:35 blah blah", "TIMEblah blah"},
		{"Version number, hex string", "123.45.68.12:345 abcd1234eeee", "UNIQ1 UNIQ2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize(tc.argument, defaultMaxClusterTextLength)

			if got != tc.want {
				t.Errorf("normalize(%s) = %s, wanted %s", tc.argument, got, tc.want)
			}
		})
	}

	// Deal with long strings separately because it requires some setup
	t.Run("Incredibly large string", func(t *testing.T) {
		// Generate an incredibly long string
		var builder strings.Builder
		builder.Grow(10 * 500_000) // Allocate enough memory (10 characters in "foobarbaz ")

		for i := 0; i < 500_000; i++ {
			builder.WriteString("foobarbaz ")
		}

		generatedString := builder.String()
		// 10*500 = (number of characters in "foobarbaz ")*(500 repetitions)
		wantString := generatedString[:10*500] + "\n...[truncated]...\n" + generatedString[:10*500]

		got := normalize(generatedString, defaultMaxClusterTextLength)

		if got != wantString {
			t.Errorf("normalize(%s) = %s, wanted %s", generatedString, wantString, got)
		}
	})
}

func TestNgramEditDist(t *testing.T) {
	argument1 := "example text"
	argument2 := "exampl text"
	want := 1
	got := ngramEditDist(argument1, argument2)

	if got != want {
		t.Errorf("ngramEditDist(%#v, %#v) = %d, wanted %d", argument1, argument2, got, want)
	}
}

// Ensure stability of ngram count digest
func TestMakeNgramCountsDigest(t *testing.T) {
	want := "eddb950347d1eb05b5d7"
	got := makeNgramCountsDigest("some string")

	if got != want {
		t.Errorf("makeNgramCountsDigest(%#v) = %#v, wanted %#v", "some string", got, want)
	}
}

// TestPodFailureClusteringSameID verifies that similar pod failure messages
// with different node names, pod names, timestamps, and line numbers
// all produce the same cluster ID.
func TestPodFailureClusteringSameID(t *testing.T) {
	// These are real examples of the same type of failure that should cluster together
	failureMessages := []string{
		`[FAILED] 1 errors:
pod pod-terminate-status-2-7 on node nodes-us-west1-a-w0fq container unexpected exit code 2: start=2025-12-10 13:51:30 +0000 UTC end=2025-12-10 13:51:30 +0000 UTC reason=Error message=
In [It] at: k8s.io/kubernetes/test/e2e/node/pods.go:1022 @ 12/10/25 13:52:29.244`,

		`[FAILED] 1 errors:
pod pod-terminate-status-2-14 on node i-0d1f2dfeb37c586ad container unexpected exit code 2: start=2025-12-10 22:30:45 +0000 UTC end=2025-12-10 22:30:45 +0000 UTC reason=Error message=
In [It] at: k8s.io/kubernetes/test/e2e/node/pods.go:548 @ 12/10/25 22:30:49.099`,

		`[FAILED] 1 errors:
pod pod-terminate-status-1-0 on node nodes-us-central1-c-1tpf container unexpected exit code 2: start=2025-12-23 14:17:13 +0000 UTC end=2025-12-23 14:17:14 +0000 UTC reason=Error message=
In [It] at: k8s.io/kubernetes/test/e2e/node/pods.go:849 @ 12/23/25 14:18:50.228`,

		`[FAILED] 1 errors:
pod pod-terminate-status-2-0 on node nodes-uksouth-2000002 container unexpected exit code 2: start=2025-12-22 01:53:07 +0000 UTC end=2025-12-22 01:53:07 +0000 UTC reason=Error message=
In [It] at: k8s.io/kubernetes/test/e2e/node/pods.go:1381 @ 12/22/25 01:54:33.96`,

		`[FAILED] 1 errors:
pod pod-terminate-status-2-5 on node kind-worker2 container unexpected exit code 2: start=2025-12-11 08:41:37 +0000 UTC end=2025-12-11 08:41:37 +0000 UTC reason=Error message=
In [It] at: k8s.io/kubernetes/test/e2e/node/pods.go:1381 @ 12/11/25 08:42:14.146`,
	}

	// Normalize all messages and compute their cluster IDs
	var clusterIDs []string
	for _, msg := range failureMessages {
		normalized := normalize(msg, defaultMaxClusterTextLength)
		clusterID := makeNgramCountsDigest(normalized)
		clusterIDs = append(clusterIDs, clusterID)
	}

	// All cluster IDs should be the same
	firstID := clusterIDs[0]
	for i, id := range clusterIDs {
		if id != firstID {
			t.Errorf("Failure message %d produced different cluster ID:\n  got:  %s\n  want: %s\n\nNormalized[0]: %s\nNormalized[%d]: %s",
				i, id, firstID,
				normalize(failureMessages[0], defaultMaxClusterTextLength),
				i, normalize(failureMessages[i], defaultMaxClusterTextLength))
		}
	}
}

// TestNormalizeNodeNames verifies that various node name formats are normalized
func TestNormalizeNodeNames(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"GKE style node name",
			"node nodes-us-west1-a-w0fq failed",
			"node UNIQ1 failed",
		},
		{
			"AWS style node name",
			"node i-0d1f2dfeb37c586ad failed",
			"node UNIQ1 failed",
		},
		{
			"kind cluster node name",
			"node kind-worker2 failed",
			"node UNIQ1 failed",
		},
		{
			"Pod name with numeric suffix",
			"pod pod-terminate-status-2-7 failed",
			"pod UNIQ1 failed",
		},
		{
			"Go file line number",
			"at: file.go:1234 error",
			"at: fileUNIQ1 error",
		},
		{
			"MM/DD/YY timestamp",
			"error @ 12/10/25 13:52:29.244",
			"error @ TIME",
		},
		{
			"Kubernetes random suffix with quote",
			`pod "test-7mhp" failed`,
			`pod "testUNIQ1 failed`, // suffix + quote consumed
		},
		{
			"Kubernetes random suffix with space",
			"container dynamicpv-bdf5 failed",
			"container dynamicpvUNIQ1failed", // suffix + space consumed
		},
		{
			"Kubernetes random suffix with slash",
			"namespaces/test-5405/pods",
			"namespaces/testUNIQ1pods", // suffix + slash consumed
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize(tc.input, defaultMaxClusterTextLength)
			if got != tc.expected {
				t.Errorf("normalize(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestCommonSpans(t *testing.T) {
	testCases := []struct {
		name     string
		argument []string
		want     []int
	}{
		{"Exact match", []string{"an exact match", "an exact match"}, []int{14}},
		{"Replaced word", []string{"some example string", "some other string"}, []int{5, 7, 7}},
		{"Deletion", []string{"a problem with a common set", "a common set"}, []int{2, 7, 1, 4, 13}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := commonSpans(tc.argument)

			// Check if the int slices are equal
			slicesAreEqual := true
			if len(tc.want) != len(got) {
				slicesAreEqual = false
			} else {
				for i := range tc.want {
					if tc.want[i] != got[i] {
						slicesAreEqual = false
						break
					}
				}
			}

			if !slicesAreEqual {
				t.Errorf("commonSpans(%#v) = %#v, wanted %#v", tc.argument, got, tc.want)
			}
		})
	}
}
