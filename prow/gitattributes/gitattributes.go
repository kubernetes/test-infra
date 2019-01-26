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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/github"
)

// Pattern defines a single gitattributes pattern.
type Pattern interface {
	// Match matches the given path to the pattern.
	Match(path string) bool
}

// Group is a logical collection of files. Check for a file's
// inclusion in the group using the Match method.
type Group struct {
	LinguistGeneratedPatterns []Pattern
}

// NewGroup reads the .gitattributes file in the root of the repository only.
func NewGroup(gitAttributesContent func() ([]byte, error)) (*Group, error) {
	g := &Group{
		LinguistGeneratedPatterns: []Pattern{},
	}

	bs, err := gitAttributesContent()
	if err != nil {
		switch err.(type) {
		case *github.FileNotFound:
			return g, nil
		default:
			return nil, fmt.Errorf("could not get .gitattributes: %v", err)
		}
	}

	if err := g.load(bytes.NewBuffer(bs)); err != nil {
		return nil, err
	}

	return g, nil
}

// Use load to read a .gitattributes file, and populate g with the commands.
// Each line in gitattributes file is of form:
//   pattern	attr1 attr2 ...
// That is, a pattern followed by an attributes list, separated by whitespaces.
func (g *Group) load(r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		// Leading and trailing whitespaces are ignored.
		l := strings.TrimSpace(s.Text())
		// Lines that begin with # are ignored.
		if l == "" || l[0] == '#' {
			continue
		}

		fs := strings.Fields(l)
		if len(fs) < 2 {
			continue
		}

		// When the pattern matches the path in question, the attributes listed on the line are given to the path.
		attributes := sets.NewString(fs[1:]...)
		if attributes.Has("linguist-generated=true") {
			p, err := parsePattern(fs[0])
			if err != nil {
				return fmt.Errorf("error parsing pattern: %v", err)
			}
			g.LinguistGeneratedPatterns = append(g.LinguistGeneratedPatterns, p)
		}
	}

	if err := s.Err(); err != nil {
		return fmt.Errorf("scan error: %v", err)
	}

	return nil
}

// IsLinguistGenerated determines whether a file, given here by its full path
// is included in the .gitattributes linguist-generated group.
// These files are excluded from language stats and suppressed in diffs.
// https://github.com/github/linguist/#generated-code
// Unmarked paths (linguist-generated=false) are not supported.
func (g *Group) IsLinguistGenerated(path string) bool {
	for _, p := range g.LinguistGeneratedPatterns {
		if p.Match(path) {
			return true
		}
	}
	return false
}
