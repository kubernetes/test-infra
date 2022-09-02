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

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/ghproxy/ghcache"
	"k8s.io/test-infra/prow/github"
)

func TestDiskCachePruning(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	o := &options{
		dir:                 cacheDir,
		maxConcurrency:      25,
		pushGatewayInterval: time.Minute,
		upstreamParsed:      &url.URL{},
		timeout:             30,
	}

	now := time.Now()
	github.TimeNow = func() time.Time { return now }

	// Five minutes so the test has sufficient time to finish
	// but also sufficient room until the app token which is
	// always valid for 10 minutes expires.
	expiryDuration := 5 * time.Minute
	roundTripper := func(r *http.Request) (*http.Response, error) {
		t.Logf("got a request for path %s", r.URL.Path)
		switch r.URL.Path {
		case "/app":
			return jsonResponse(github.App{Slug: "app-slug"}, 200)
		case "/app/installations":
			return jsonResponse([]github.AppInstallation{{Account: github.User{Login: "org"}}}, 200)
		case "/app/installations/0/access_tokens":
			return jsonResponse(github.AppInstallationToken{Token: "abc", ExpiresAt: now.Add(expiryDuration)}, 201)
		case "/repos/org/repo/git/refs/dev":
			return jsonResponse(github.GetRefResult{}, 200)
		default:
			return nil, fmt.Errorf("got unexpected request for %s", r.URL.Path)
		}
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	server := httptest.NewServer(proxy(o, httpRoundTripper(roundTripper), time.Hour))
	t.Cleanup(server.Close)
	_, _, client, err := github.NewClientFromOptions(logrus.Fields{}, github.ClientOptions{
		MaxRetries:      1,
		Censor:          func(b []byte) []byte { return b },
		AppID:           "123",
		AppPrivateKey:   func() *rsa.PrivateKey { return rsaKey },
		Bases:           []string{server.URL},
		GraphqlEndpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to construct github client: %v", err)
	}

	if _, err := client.GetRef("org", "repo", "dev"); err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}

	numberPartitions, err := getNumberOfCachePartitions(cacheDir)
	if err != nil {
		t.Fatalf("failed to get number of cache paritions: %v", err)
	}
	if numberPartitions != 2 {
		t.Fatalf("expected two cache paritions, one for the app and one for the app installation, got %d", numberPartitions)
	}

	ghcache.Prune(cacheDir, func() time.Time { return now.Add(expiryDuration).Add(time.Second) })

	numberPartitions, err = getNumberOfCachePartitions(cacheDir)
	if err != nil {
		t.Fatalf("failed to get number of cache paritions: %v", err)
	}
	if numberPartitions != 1 {
		t.Errorf("expected one cache partition for the app as the one for the installation should be cleaned up, got  %d", numberPartitions)
	}
}

func getNumberOfCachePartitions(cacheDir string) (int, error) {
	var result int
	for _, suffix := range []string{"temp", "data"} {
		entries, err := os.ReadDir(path.Join(cacheDir, suffix))
		if err != nil {
			return result, fmt.Errorf("faield to list: %w", err)
		}
		if result == 0 {
			result = len(entries)
			continue
		}
		if n := len(entries); n != result {
			return result, fmt.Errorf("temp and datadir don't have the same number of partitions: %d vs %d", result, n)
		}
	}

	return result, nil
}

type httpRoundTripper func(*http.Request) (*http.Response, error)

func (rt httpRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

func jsonResponse(body interface{}, statusCode int) (*http.Response, error) {
	serialized, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: statusCode, Body: io.NopCloser(bytes.NewBuffer(serialized)), Header: http.Header{}}, nil
}
