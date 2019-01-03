/*
Copyright 2016 The Kubernetes Authors.

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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

const gitAttributesFile = ".gitattributes"

// ghFileClient scopes to the only relevant functionality we require of a github client.
type ghFileClient interface {
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

// Group is a logical collection of files. Check for a file's
// inclusion in the group using the Match method.
type Group struct {
	LinguistGeneratedPatterns []gitignore.Pattern
}

// NewGroup reads the .gitattributes file in the root of the repository only.
// The rules by which patterns matche paths are the same as in .gitignore files.
func NewGroup(gc ghFileClient, owner, repo, sha string) (*Group, error) {
	g := &Group{
		LinguistGeneratedPatterns: []gitignore.Pattern{},
	}

	bs, err := gc.GetFile(owner, repo, gitAttributesFile, sha)
	if err != nil {
		switch err.(type) {
		case *github.FileNotFound:
			return g, nil
		default:
			return nil, fmt.Errorf("could not get %s: %v", gitAttributesFile, err)
		}
	}

	if err := g.load(bytes.NewBuffer(bs)); err != nil {
		return nil, err
	}

	return g, nil
}

// Use load to read a .gitattributes file, and populate g with the commands.
func (g *Group) load(r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || l[0] == '#' {
			// Ignore comments and empty lines.
			continue
		}

		fs := strings.Fields(l)
		if len(fs) < 2 {
			continue
		}

		attributes := sets.NewString(fs[1:]...)
		if attributes.Has("linguist-generated=true") {
			g.LinguistGeneratedPatterns = append(g.LinguistGeneratedPatterns, gitignore.ParsePattern(fs[0], nil))
		}
	}

	if err := s.Err(); err != nil {
		return fmt.Errorf("scan error: %v", err)
	}

	return nil
}

// IsLinguistGenerated determines whether a file, given here by its full path
// is included in the .gitattributes linguist-generated group.
func (g *Group) IsLinguistGenerated(path string) bool {
	// delegate to gitignore Matcher
	return gitignore.NewMatcher(g.LinguistGeneratedPatterns).Match(strings.Split(path, "/"), false)
}
