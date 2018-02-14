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

package config

import (
	"encoding/base64"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
)

func TestDecode(t *testing.T) {
	redirectURL := "https://example.com"
	finalRedirectURL := "https://some-mock-url"
	scopes := []string{"email", "password", "mocks-scope"}
	mockConfig := &GithubOAuthConfig{
		RedirectURL:      redirectURL,
		FinalRedirectURL: finalRedirectURL,
		Scopes:           scopes,
	}

	var decodedScopes []string
	for _, val := range scopes {
		decodedScopes = append(decodedScopes, base64.StdEncoding.EncodeToString([]byte(val)))
	}

	decodedMockConfig := &GithubOAuthConfig{
		RedirectURL:      base64.StdEncoding.EncodeToString([]byte(redirectURL)),
		FinalRedirectURL: base64.StdEncoding.EncodeToString([]byte(finalRedirectURL)),
		Scopes:           decodedScopes,
	}

	decodedMockConfig.Decode()
	if !equality.Semantic.DeepEqual(mockConfig, decodedMockConfig) {
		t.Errorf("Decoded returns wrong result. Got: %v, expected: %v", decodedMockConfig, mockConfig)
	}
}
