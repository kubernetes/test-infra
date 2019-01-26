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
	"fmt"
	"path/filepath"
	"strings"
)

const (
	negativePrefix = "!"
	patternDirSep  = "/"
	zeroToManyDirs = "**"
)

type pattern struct {
	pattern []string
	isPath  bool
}

// parsePattern parses a gitattributes pattern string into the Pattern structure.
// The rules by which the pattern matches paths are the same as in .gitignore files (see https://git-scm.com/docs/gitignore), with a few exceptions:
//   - negative patterns are forbidden
//   - patterns that match a directory do not recursively match paths inside that directory
// https://git-scm.com/docs/gitattributes
func parsePattern(p string) (Pattern, error) {
	res := pattern{}

	// negative patterns are forbidden
	if strings.HasPrefix(p, negativePrefix) {
		return nil, fmt.Errorf("negative patterns are forbidden: <%s>", p)
	}

	// trailing spaces are ignored unless they are quoted with backslash
	if !strings.HasSuffix(p, `\ `) {
		p = strings.TrimRight(p, " ")
	}

	// patterns that match a directory do not recursively match paths inside that directory
	if strings.HasSuffix(p, patternDirSep) {
		return nil, fmt.Errorf("directory patterns are not matched recursively, use path/** instead: <%s>", p)
	}

	if strings.Contains(p, patternDirSep) {
		res.isPath = true
	}

	res.pattern = strings.Split(p, patternDirSep)
	return &res, nil
}

func (p *pattern) Match(path string) bool {
	pathElements := strings.Split(path, "/")
	if p.isPath && p.pathMatch(pathElements) {
		return true
	} else if !p.isPath && p.nameMatch(pathElements) {
		return true
	}
	return false
}

func (p *pattern) nameMatch(path []string) bool {
	for _, name := range path {
		if match, err := filepath.Match(p.pattern[0], name); err != nil {
			return false
		} else if !match {
			continue
		}
		return true
	}
	return false
}

func (p *pattern) pathMatch(path []string) bool {
	matched := false
	canTraverse := false
	for i, pattern := range p.pattern {
		if pattern == "" {
			canTraverse = false
			continue
		}
		if pattern == zeroToManyDirs {
			if i == len(p.pattern)-1 {
				break
			}
			canTraverse = true
			continue
		}
		if strings.Contains(pattern, zeroToManyDirs) {
			return false
		}
		if len(path) == 0 {
			return false
		}
		if canTraverse {
			canTraverse = false
			for len(path) > 0 {
				e := path[0]
				path = path[1:]
				if match, err := filepath.Match(pattern, e); err != nil {
					return false
				} else if match {
					matched = true
					break
				} else if len(path) == 0 {
					// if nothing left then fail
					matched = false
				}
			}
		} else {
			if match, err := filepath.Match(pattern, path[0]); err != nil || !match {
				return false
			}
			matched = true
			path = path[1:]
		}
	}
	return matched
}
