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
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

// ErrTagList is returned when the OCI tag list request fails.
var ErrTagList = errors.New("tag list request failed")

// registryResolver implements ImageResolver by querying OCI Distribution
// tag list endpoints. It caches tag lists per repository.
type registryResolver struct {
	client *http.Client
	mu     sync.Mutex
	cache  map[string][]string
}

// NewRegistryResolver creates an ImageResolver that checks tags against
// the OCI Distribution API. Pass nil to use http.DefaultClient.
func NewRegistryResolver(client *http.Client) ImageResolver {
	if client == nil {
		client = http.DefaultClient
	}

	return &registryResolver{
		client: client,
		mu:     sync.Mutex{},
		cache:  make(map[string][]string),
	}
}

// imageRef holds the parsed components of a container image reference.
type imageRef struct {
	registry string // e.g., "gcr.io" or "us-central1-docker.pkg.dev"
	repo     string // e.g., "k8s-staging-test-infra/kubekins-e2e"
	tag      string // e.g., "v20260205-38cfa9523f-1.34"
}

// parseImageRef parses "registry/repo:tag" into components.
func parseImageRef(image string) (imageRef, bool) {
	colonIdx := strings.LastIndex(image, ":")
	if colonIdx < 0 {
		return imageRef{registry: "", repo: "", tag: ""}, false
	}

	repoPath := image[:colonIdx]
	tag := image[colonIdx+1:]

	registry, repo, ok := strings.Cut(repoPath, "/")
	if !ok {
		return imageRef{registry: "", repo: "", tag: ""}, false
	}

	return imageRef{
		registry: registry,
		repo:     repo,
		tag:      tag,
	}, true
}

// tagSuffixPattern matches tags like "vYYYYMMDD-hexhash-suffix".
// Submatch index 1 is the suffix portion (e.g., "1.34" or "master").
var tagSuffixPattern = regexp.MustCompile(`^v\d{8}-[0-9a-f]+-(.+)$`)

// tagSuffixSubmatch is the expected number of submatches from tagSuffixPattern.
const tagSuffixSubmatch = 2

// tagSuffix extracts the suffix portion of a tag (e.g., "1.34" from
// "v20260205-38cfa9523f-1.34").
func tagSuffix(tag string) string {
	m := tagSuffixPattern.FindStringSubmatch(tag)
	if len(m) < tagSuffixSubmatch {
		return ""
	}

	return m[1]
}

func (r *registryResolver) Resolve(
	ctx context.Context, image string,
) (string, error) {
	ref, ok := parseImageRef(image)
	if !ok {
		return image, nil
	}

	tags, err := r.getTags(ctx, ref)
	if err != nil {
		return "", err
	}

	if slices.Contains(tags, ref.tag) {
		return image, nil
	}

	// Tag doesn't exist; find the latest tag with the same suffix.
	wantSuffix := tagSuffix(ref.tag)
	if wantSuffix == "" {
		return image, nil
	}

	var candidates []string

	for _, t := range tags {
		if tagSuffix(t) == wantSuffix {
			candidates = append(candidates, t)
		}
	}

	if len(candidates) == 0 {
		return image, nil
	}

	sort.Strings(candidates)

	bestTag := candidates[len(candidates)-1]

	return image[:strings.LastIndex(image, ":")+1] + bestTag, nil
}

// tagListResponse is the JSON structure returned by the OCI tag list endpoint.
type tagListResponse struct {
	Tags []string `json:"tags"`
}

func (r *registryResolver) getTags(
	ctx context.Context, ref imageRef,
) ([]string, error) {
	cacheKey := ref.registry + "/" + ref.repo

	r.mu.Lock()
	if tags, ok := r.cache[cacheKey]; ok {
		r.mu.Unlock()

		return tags, nil
	}

	r.mu.Unlock()

	url := fmt.Sprintf(
		"https://%s/v2/%s/tags/list", ref.registry, ref.repo,
	)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, url, http.NoBody,
	)
	if err != nil {
		return nil, fmt.Errorf("creating tag list request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching tag list from %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"%w: %s returned %d", ErrTagList, url, resp.StatusCode,
		)
	}

	var tlr tagListResponse
	if err := json.NewDecoder(resp.Body).Decode(&tlr); err != nil {
		return nil, fmt.Errorf(
			"decoding tag list from %s: %w", url, err,
		)
	}

	r.mu.Lock()
	r.cache[cacheKey] = tlr.Tags
	r.mu.Unlock()

	return tlr.Tags, nil
}
