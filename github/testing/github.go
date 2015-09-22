/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package testing

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/google/go-github/github"
)

// InitTest will return a github.Client which will talk to the httptest.Server,
// to retrieve information from the http.ServeMux
func InitTest() (*github.Client, *httptest.Server, *http.ServeMux) {
	// test server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	// github client configured to use test server
	client := github.NewClient(nil)
	url, _ := url.Parse(server.URL)
	client.BaseURL = url
	client.UploadURL = url

	return client, server, mux
}
