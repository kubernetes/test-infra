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

// Package source contains functions that help with Gerrit source control
// specific logics.
package source

import (
	"fmt"
	"strings"
)

// IsGerritOrg tells whether the org is a Gerrit org or not. It returns true
// when the org string starts with https://.
func IsGerritOrg(org string) bool {
	return strings.HasPrefix(org, "https://") || strings.HasPrefix(org, "http://")
}

// CloneURIFromOrgRepo returns normalized cloneURI from org and repo. The
// returns cloneURI will always have https:// or http:// prefix, and there is no
// trailing slash at the end.
func CloneURIFromOrgRepo(org, repo string) string {
	return NormalizeCloneURI(orgRepo(org, repo))
}

// NormalizeOrg returns normalized org. It ensures that org always has https://
// or http:// prefix, and there is no trailing slash at the end. This function
// should be used everywhere that Gerrit org is referenced.
func NormalizeOrg(org string) string {
	return strings.TrimRight(ensuresHTTPSPrefix(org), "/")
}

// NormalizeCloneURI returns normalized cloneURI. It ensures that cloneURI
// always has https:// or http:// prefix, and there is no trailing slash at the
// end. This function should be used everywhere that Gerrit cloneURI is
// referenced.
func NormalizeCloneURI(cloneURI string) string {
	return strings.TrimRight(ensuresHTTPSPrefix(cloneURI), "/")
}

// OrgRepoFromCloneURI returns org and repo from cloneURI. The returned org
// always has https:// or http:// prefix even if cloneURI doesn't have it.
func OrgRepoFromCloneURI(cloneURI string) (string, string, error) {
	scheme := "https://"
	if strings.HasPrefix(cloneURI, "http://") {
		scheme = "http://"
	}
	cloneURIWithoutPrefix := TrimHTTPSPrefix(cloneURI)
	var org, repo string
	parts := strings.SplitN(cloneURIWithoutPrefix, "/", 2)
	if len(parts) != 2 {
		return org, repo, fmt.Errorf("should have 2 parts: %v", parts)
	}
	return NormalizeOrg(scheme + parts[0]), strings.TrimRight(parts[1], "/"), nil
}

func ensuresHTTPSPrefix(in string) string {
	scheme := "https://"
	if strings.HasPrefix(in, "http://") {
		scheme = "http://"
	}
	return fmt.Sprintf("%s%s", scheme, strings.Trim(TrimHTTPSPrefix(in), "/"))
}

// TrimHTTPSPrefix trims https:// and http:// from input, also remvoes all
// trailing slashes from the end.
func TrimHTTPSPrefix(in string) string {
	in = strings.TrimPrefix(in, "https://")
	in = strings.TrimPrefix(in, "http://")
	return strings.TrimRight(in, "/")
}

// orgRepo returns <org>/<repo>, removes all extra slashs.
func orgRepo(org, repo string) string {
	org = strings.Trim(org, "/")
	repo = strings.Trim(repo, "/")
	return org + "/" + repo
}

// CodeRootURL converts code review URL into source code URL, simply
// trimming the `-review` suffix from the name of the org.
//
// Gerrit URL for sourcecode looks like
// https://android.googlesource.com, and the code review URL looks like
// https://android-review.googlesource.com/c/platform/frameworks/support/+/2260382.
func CodeRootURL(reviewURL string) (string, error) {
	orgParts := strings.Split(reviewURL, ".")
	if !strings.HasSuffix(orgParts[0], "-review") {
		return "", fmt.Errorf("cannot find '-review' suffix from the first part of url %v", orgParts)
	}
	orgParts[0] = strings.TrimSuffix(orgParts[0], "-review")
	return strings.Join(orgParts, "."), nil
}
