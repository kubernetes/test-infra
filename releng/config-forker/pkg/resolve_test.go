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

package forker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestParseImageRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		image    string
		wantOK   bool
		wantRef  imageRef
	}{
		{
			name:   "standard GCR image",
			image:  "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20260205-38cfa9523f-1.34",
			wantOK: true,
			wantRef: imageRef{
				registry: "gcr.io",
				repo:     "k8s-staging-test-infra/kubekins-e2e",
				tag:      "v20260205-38cfa9523f-1.34",
			},
		},
		{
			name:   "artifact registry image",
			image:  "us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:v20260209-abc123-master",
			wantOK: true,
			wantRef: imageRef{
				registry: "us-central1-docker.pkg.dev",
				repo:     "k8s-staging-test-infra/images/kubekins-e2e",
				tag:      "v20260209-abc123-master",
			},
		},
		{
			name:    "no tag",
			image:   "gcr.io/k8s-staging-test-infra/kubekins-e2e",
			wantOK:  false,
			wantRef: imageRef{registry: "", repo: "", tag: ""},
		},
		{
			name:    "no slash",
			image:   "ubuntu:latest",
			wantOK:  false,
			wantRef: imageRef{registry: "", repo: "", tag: ""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ref, ok := parseImageRef(test.image)
			if ok != test.wantOK {
				t.Fatalf("parseImageRef(%q) ok = %v, want %v", test.image, ok, test.wantOK)
			}

			if ok && ref != test.wantRef {
				t.Errorf("parseImageRef(%q) = %+v, want %+v", test.image, ref, test.wantRef)
			}
		})
	}
}

func TestTagSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag      string
		expected string
	}{
		{"v20260205-38cfa9523f-1.34", "1.34"},
		{"v20260205-38cfa9523f-master", "master"},
		{"latest", ""},
		{"latest-1.34", ""},
		{"v20260205-UPPER-1.34", ""},
	}

	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			t.Parallel()

			if got := tagSuffix(test.tag); got != test.expected {
				t.Errorf("tagSuffix(%q) = %q, want %q", test.tag, got, test.expected)
			}
		})
	}
}

func newTestServer(t *testing.T, tags []string, callCount *atomic.Int32) *httptest.Server {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if callCount != nil {
			callCount.Add(1)
		}

		writer.Header().Set("Content-Type", "application/json")

		resp := tagListResponse{Tags: tags}
		if err := json.NewEncoder(writer).Encode(resp); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

func testImage(serverURL, tag string) string {
	return serverURL[len("https://"):] + "/repo/image:" + tag
}

func TestRegistryResolver_ExactTagExists(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, []string{
		"v20260205-38cfa9523f-master",
		"v20260205-38cfa9523f-1.34",
	}, nil)

	resolver := NewRegistryResolver(server.Client())
	image := testImage(server.URL, "v20260205-38cfa9523f-1.34")

	got, err := resolver.Resolve(context.Background(), image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != image {
		t.Errorf("Resolve() = %q, want %q (unchanged)", got, image)
	}
}

func TestRegistryResolver_FallbackToLatest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, []string{
		"v20260101-aaa1111111-1.34",
		"v20260115-bbb2222222-1.34",
		"v20260205-38cfa9523f-master",
	}, nil)

	resolver := NewRegistryResolver(server.Client())
	image := testImage(server.URL, "v20260205-38cfa9523f-1.34")
	expected := testImage(server.URL, "v20260115-bbb2222222-1.34")

	got, err := resolver.Resolve(context.Background(), image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != expected {
		t.Errorf("Resolve() = %q, want %q", got, expected)
	}
}

func TestRegistryResolver_NoFallback(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, []string{
		"v20260205-38cfa9523f-master",
	}, nil)

	resolver := NewRegistryResolver(server.Client())
	image := testImage(server.URL, "v20260205-38cfa9523f-1.34")

	got, err := resolver.Resolve(context.Background(), image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != image {
		t.Errorf("Resolve() = %q, want %q (unchanged)", got, image)
	}
}

func TestRegistryResolver_UnparseableImage(t *testing.T) {
	t.Parallel()

	resolver := NewRegistryResolver(nil)

	got, err := resolver.Resolve(context.Background(), "ubuntu:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "ubuntu:latest" {
		t.Errorf("Resolve() = %q, want %q", got, "ubuntu:latest")
	}
}

func TestRegistryResolver_CacheHit(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := newTestServer(t, []string{
		"v20260205-38cfa9523f-1.34",
	}, &calls)

	resolver := NewRegistryResolver(server.Client())
	image := testImage(server.URL, "v20260205-38cfa9523f-1.34")

	for range 3 {
		if _, err := resolver.Resolve(context.Background(), image); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call, got %d", got)
	}
}

func TestRegistryResolver_HTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))

	t.Cleanup(server.Close)

	resolver := NewRegistryResolver(server.Client())
	image := testImage(server.URL, "v20260205-38cfa9523f-1.34")

	_, err := resolver.Resolve(context.Background(), image)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRegistryResolver_CancelledContext(t *testing.T) {
	t.Parallel()

	resolver := NewRegistryResolver(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := resolver.Resolve(ctx, "gcr.io/some/repo:v20260205-38cfa9523f-1.34")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestResolveImage_NilResolver(t *testing.T) {
	t.Parallel()

	got, err := resolveImage(context.Background(), nil, "some-image:tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "some-image:tag" {
		t.Errorf("resolveImage() = %q, want %q", got, "some-image:tag")
	}
}

func TestResolveImage_VariableRef(t *testing.T) {
	t.Parallel()

	resolver := NewRegistryResolver(nil)
	image := "${kubekins_e2e_image}-master"

	got, err := resolveImage(context.Background(), resolver, image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != image {
		t.Errorf("resolveImage() = %q, want %q", got, image)
	}
}
