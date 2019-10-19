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

package gitattributes

import (
	"bytes"
	"testing"
)

func TestLoad(t *testing.T) {
	var cases = []struct {
		name                        string
		src                         string
		nbLinguistGeneratedPatterns int
		expectError                 bool
	}{
		{
			name: "kubernetes",
			src: `hack/verify-flags/known-flags.txt merge=union

**/zz_generated.*.go linguist-generated=true
**/types.generated.go linguist-generated=true
**/generated.pb.go linguist-generated=true
**/generated.proto
**/types_swagger_doc_generated.go linguist-generated=true
docs/api-reference/** linguist-generated=true
api/swagger-spec/*.json linguist-generated=true
api/openapi-spec/*.json linguist-generated=true`,
			nbLinguistGeneratedPatterns: 7,
		},
		{
			name: "mholt/caddy",
			src: `# shell scripts should not use tabs to indent!
*.bash    text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2
*.sh      text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2

# files for systemd (shell-similar)
*.path    text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2
*.service text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2
*.timer   text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2

# go fmt will enforce this, but in case a user has not called "go fmt" allow GIT to catch this:
*.go      text eol=lf core.whitespace whitespace=indent-with-non-tab,trailing-space,tabwidth=4

*.yml     text eol=lf core.whitespace whitespace=tab-in-indent,trailing-space,tabwidth=2
.git*     text eol=auto core.whitespace whitespace=trailing-space`,
		},
		{
			name: "opencontainers/runtime-spec",
			src: `# https://tools.ietf.org/html/rfc5545#section-3.1
*.ics text eol=crlf`,
		},
		{
			name: "test-infra/prow/cmd/phony/examples",
			src: `# Treat webhook fixtures as generated so they don't clutter PRs
*.json linguist-generated=true`,
			nbLinguistGeneratedPatterns: 1,
		},
		{
			name:        "wrong pattern",
			src:         `abc/ linguist-generated=true`,
			expectError: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &Group{
				LinguistGeneratedPatterns: []Pattern{},
			}
			if err := g.load(bytes.NewBufferString(c.src)); err != nil && !c.expectError {
				t.Fatalf("load error: %v", err)
			}
			if got := len(g.LinguistGeneratedPatterns); got != c.nbLinguistGeneratedPatterns {
				t.Fatalf("len(g.LinguistGeneratedPatterns) mismatch: got %d, want %d", got, c.nbLinguistGeneratedPatterns)
			}
		})
	}
}

func TestIsLinguistGenerated(t *testing.T) {
	var src = `hack/verify-flags/known-flags.txt merge=union

**/zz_generated.*.go linguist-generated=true
**/types.generated.go linguist-generated=true
**/generated.pb.go linguist-generated=true
**/generated.proto
**/types_swagger_doc_generated.go linguist-generated=true
docs/api-reference/** linguist-generated=true
api/swagger-spec/*.json linguist-generated=true
api/openapi-spec/*.json linguist-generated=true`
	var cases = []struct {
		name string
		path string
		want bool
	}{
		{
			name: "known-flags.txt",
			path: "hack/verify-flags/known-flags.txt",
			want: false,
		},
		{
			name: "generated.proto",
			path: "a/generated.proto",
			want: false,
		},
		{
			name: "generated.pb.go",
			path: "a/b/generated.pb.go",
			want: true,
		},
		{
			name: "abc.xml",
			path: "docs/api-reference/a/b/c/abc.xml",
			want: true,
		},
		{
			name: "abc.json",
			path: "api/openapi-spec/abc.json",
			want: true,
		},
	}
	for _, c := range cases {
		g := &Group{
			LinguistGeneratedPatterns: []Pattern{},
		}
		if err := g.load(bytes.NewBufferString(src)); err != nil {
			t.Fatalf("load error: %v", err)
		}
		t.Run(c.name, func(t *testing.T) {
			if got := g.IsLinguistGenerated(c.path); got != c.want {
				t.Fatalf("IsLinguistGenerated mismatch: got %t, want %t", got, c.want)
			}
		})
	}
}
