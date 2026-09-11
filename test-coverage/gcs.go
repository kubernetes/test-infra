/*
Copyright The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// gcsBucket is the public GCS bucket which holds the Prow job results.
const gcsBucket = "kubernetes-ci-logs"

// gcsClient talks to the public "storage.googleapis.com" REST API. It does
// not require any credentials because the bucket is world-readable.
type gcsClient struct {
	httpClient *http.Client
}

func newGCSClient() *gcsClient {
	return &gcsClient{httpClient: http.DefaultClient}
}

// listObjectsResponse is the subset of the GCS JSON API "objects.list"
// response that we care about.
type listObjectsResponse struct {
	Prefixes []string `json:"prefixes"`
	Items    []struct {
		Name string `json:"name"`
	} `json:"items"`
	NextPageToken string `json:"nextPageToken"`
}

// listPrefixes lists the immediate "directories" (common prefixes) below
// prefix, using "/" as delimiter. prefix must end in "/".
func (c *gcsClient) listPrefixes(prefix string) ([]string, error) {
	var result []string
	pageToken := ""
	for {
		u := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o?prefix=%s&delimiter=%s&fields=%s",
			url.QueryEscape(gcsBucket),
			url.QueryEscape(prefix),
			url.QueryEscape("/"),
			url.QueryEscape("prefixes,nextPageToken"),
		)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var resp listObjectsResponse
		if err := c.getJSON(u, &resp); err != nil {
			return nil, fmt.Errorf("listing %q: %w", prefix, err)
		}
		result = append(result, resp.Prefixes...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return result, nil
}

// listFiles lists the object names (not sub-"directories") directly below
// prefix, using "/" as delimiter. prefix must end in "/".
func (c *gcsClient) listFiles(prefix string) ([]string, error) {
	var result []string
	pageToken := ""
	for {
		u := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o?prefix=%s&delimiter=%s&fields=%s",
			url.QueryEscape(gcsBucket),
			url.QueryEscape(prefix),
			url.QueryEscape("/"),
			url.QueryEscape("items(name),nextPageToken"),
		)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var resp listObjectsResponse
		if err := c.getJSON(u, &resp); err != nil {
			return nil, fmt.Errorf("listing %q: %w", prefix, err)
		}
		for _, item := range resp.Items {
			result = append(result, item.Name)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return result, nil
}

// getObject downloads the content of a single object.
func (c *gcsClient) getObject(name string) ([]byte, error) {
	u := "https://storage.googleapis.com/" + gcsBucket + "/" + strings.TrimPrefix(name, "/")
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("downloading %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %q: unexpected status %s", name, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *gcsClient) getJSON(u string, out interface{}) error {
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %s: %s", resp.Status, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
