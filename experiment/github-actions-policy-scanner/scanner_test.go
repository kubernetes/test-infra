/*
Copyright 2026 The Kubernetes Authors.

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
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/github"
)

func TestClassifyUsesValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		value         string
		wantViolation bool
		wantMessage   string
	}{
		{
			name:          "pinned sha",
			value:         "actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11",
			wantViolation: false,
		},
		{
			name:          "tag ref",
			value:         "actions/checkout@v4",
			wantViolation: true,
			wantMessage:   "mutable ref; expected 40-character commit SHA",
		},
		{
			name:          "branch ref",
			value:         "actions/checkout@main",
			wantViolation: true,
			wantMessage:   "mutable ref; expected 40-character commit SHA",
		},
		{
			name:          "short sha",
			value:         "actions/checkout@abc1234",
			wantViolation: true,
			wantMessage:   "mutable ref; expected 40-character commit SHA",
		},
		{
			name:          "missing ref",
			value:         "actions/checkout",
			wantViolation: true,
			wantMessage:   "missing ref; expected 40-character commit SHA",
		},
		{
			name:          "trailing at sign",
			value:         "actions/checkout@",
			wantViolation: true,
			wantMessage:   "missing ref; expected 40-character commit SHA",
		},
		{
			name:          "dynamic ref",
			value:         "actions/checkout@${{ matrix.ref }}",
			wantViolation: true,
			wantMessage:   "dynamic ref; cannot verify immutable SHA pin",
		},
		{
			name:          "local action",
			value:         "./.github/actions/foo",
			wantViolation: false,
		},
		{
			name:          "docker ref",
			value:         "docker://alpine:3.20",
			wantViolation: false,
		},
		{
			name:          "reusable workflow sha",
			value:         "kubernetes/test-infra/.github/workflows/reusable.yaml@0123456789abcdef0123456789abcdef01234567",
			wantViolation: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := classifyUsesValue(tc.value)
			if ok != tc.wantViolation {
				t.Fatalf("classifyUsesValue(%q) violation = %t, want %t", tc.value, ok, tc.wantViolation)
			}
			if got.Message != tc.wantMessage {
				t.Fatalf("classifyUsesValue(%q) message = %q, want %q", tc.value, got.Message, tc.wantMessage)
			}
		})
	}
}

func TestScanUsesWorkflow(t *testing.T) {
	t.Parallel()

	content := `
jobs:
  build:
    steps:
    - uses: "actions/checkout@v4"
    - uses: ./.github/actions/local
    - uses: actions/setup-go@b4c2f1f0c5d7d0ed52e0e6f24d4f5f1f4b7b1234
  reusable:
    uses: kubernetes/test-infra/.github/workflows/reusable.yaml@main
  shell:
    steps:
    - name: shell text only
      run: |
        uses: actions/cache@v4
`

	findings, err := scanUses("kubernetes", "test-infra", ".github/workflows/test.yaml", content)
	if err != nil {
		t.Fatalf("scanUses() returned error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("scanUses() returned %d findings, want 2", len(findings))
	}

	if findings[0].Line != 5 || findings[0].Uses != "actions/checkout@v4" {
		t.Fatalf("first finding = %+v, want line 5 uses actions/checkout@v4", findings[0])
	}
	if findings[1].Line != 9 || findings[1].Uses != "kubernetes/test-infra/.github/workflows/reusable.yaml@main" {
		t.Fatalf("second finding = %+v, want line 9 reusable workflow violation", findings[1])
	}
}

func TestScanUsesCompositeAction(t *testing.T) {
	t.Parallel()

	content := `
runs:
  using: composite
  steps:
  - uses: "actions/checkout@v4"
  - uses: ./.github/actions/local
`

	findings, err := scanUses("kubernetes", "test-infra", ".github/actions/example/action.yaml", content)
	if err != nil {
		t.Fatalf("scanUses() returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("scanUses() returned %d findings, want 1", len(findings))
	}
	if findings[0].Line != 5 || findings[0].Uses != "actions/checkout@v4" {
		t.Fatalf("finding = %+v, want line 5 uses actions/checkout@v4", findings[0])
	}
}

func TestScanUsesInvalidYAML(t *testing.T) {
	t.Parallel()

	_, err := scanUses("kubernetes", "test-infra", ".github/workflows/test.yaml", "jobs: [")
	if err == nil {
		t.Fatal("scanUses() error = nil, want parse error")
	}
}

func TestIsCandidateFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path  string
		wants bool
	}{
		{path: ".github/workflows/test.yaml", wants: true},
		{path: ".github/workflows/nested/test.yml", wants: true},
		{path: ".github/actions/example/action.yaml", wants: true},
		{path: ".github/ISSUE_TEMPLATE/bug-report.md", wants: false},
		{path: "docs/action.yaml", wants: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := isCandidateFile(tc.path); got != tc.wants {
				t.Fatalf("isCandidateFile(%q) = %t, want %t", tc.path, got, tc.wants)
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	if got, ok := retryAfter("120"); !ok || got != 120*time.Second {
		t.Fatalf("retryAfter integer = (%v, %t), want (120s, true)", got, ok)
	}

	headerValue := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	if got, ok := retryAfter(headerValue); !ok || got <= 0 {
		t.Fatalf("retryAfter http-date = (%v, %t), want positive duration", got, ok)
	}

	if got, ok := retryAfter(""); ok || got != 0 {
		t.Fatalf("retryAfter empty = (%v, %t), want (0, false)", got, ok)
	}
}

func TestLowRateSleep(t *testing.T) {
	t.Parallel()

	resp := &github.Response{
		Rate: github.Rate{
			Remaining: 10,
			Reset:     github.Timestamp{Time: time.Now().Add(2 * time.Second)},
		},
	}
	if got := lowRateSleep(resp); got <= 2*time.Second {
		t.Fatalf("lowRateSleep() = %v, want > 2s", got)
	}

	resp.Rate.Remaining = 100
	if got := lowRateSleep(resp); got != 0 {
		t.Fatalf("lowRateSleep() with plenty remaining = %v, want 0", got)
	}
}

func TestRetryDelay(t *testing.T) {
	t.Parallel()

	if got, ok := retryDelay(&github.ErrorResponse{Response: &http.Response{
		StatusCode: http.StatusServiceUnavailable,
	}}, 0); !ok || got != time.Second {
		t.Fatalf("retryDelay 5xx attempt0 = (%v, %t), want (1s, true)", got, ok)
	}

	abuseDelay, ok := retryDelay(&github.AbuseRateLimitError{}, 2)
	if !ok || abuseDelay != 120*time.Second {
		t.Fatalf("retryDelay abuse attempt2 = (%v, %t), want (120s, true)", abuseDelay, ok)
	}

	retryHeaderDelay, ok := retryDelay(&github.ErrorResponse{Response: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"30"}},
	}}, 0)
	if !ok || retryHeaderDelay != 30*time.Second {
		t.Fatalf("retryDelay retry-after = (%v, %t), want (30s, true)", retryHeaderDelay, ok)
	}

	rateDelay, ok := retryDelay(&github.RateLimitError{
		Rate: github.Rate{
			Reset: github.Timestamp{Time: time.Now().Add(2 * time.Second)},
		},
	}, 0)
	if !ok || rateDelay <= 2*time.Second {
		t.Fatalf("retryDelay rate-limit = (%v, %t), want > 2s", rateDelay, ok)
	}

	if got, ok := retryDelay(&github.ErrorResponse{Response: &http.Response{
		StatusCode: http.StatusServiceUnavailable,
	}}, maxRetries); ok || got != 0 {
		t.Fatalf("retryDelay exhausted = (%v, %t), want (0, false)", got, ok)
	}
}

func TestNormalizeGitHubAPIEndpoint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       string
		want      string
		wantError bool
	}{
		{
			name: "api github",
			raw:  "https://api.github.com",
			want: "https://api.github.com/",
		},
		{
			name: "ghproxy root",
			raw:  "http://ghproxy.test-pods.svc.cluster.local",
			want: "http://ghproxy.test-pods.svc.cluster.local/",
		},
		{
			name:      "missing scheme",
			raw:       "api.github.com",
			wantError: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeGitHubAPIEndpoint(tc.raw)
			if tc.wantError {
				if err == nil {
					t.Fatalf("normalizeGitHubAPIEndpoint(%q) error = nil, want error", tc.raw)
				}
				return
			}

			if err != nil {
				t.Fatalf("normalizeGitHubAPIEndpoint(%q) returned error: %v", tc.raw, err)
			}
			if got.String() != tc.want {
				t.Fatalf("normalizeGitHubAPIEndpoint(%q) = %q, want %q", tc.raw, got.String(), tc.want)
			}
		})
	}
}
