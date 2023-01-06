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

package simplifypath

import (
	"strings"

	"github.com/sirupsen/logrus"
)

const unmatchedPath = "unmatched"

// Simplifier knows how to simplify a path
type Simplifier interface {
	Simplify(path string) string
}

// NewSimplifier builds a new simplifier for the tree
func NewSimplifier(tree Node) Simplifier {
	return &simplifier{
		tree: tree,
	}
}

type simplifier struct {
	tree Node
}

// Simplify returns a variable-free path that can be used as label for prometheus metrics
func (s *simplifier) Simplify(path string) string {
	splitPath := strings.Split(path, "/")
	resolvedPath, matches := resolve(s.tree, splitPath)
	if !matches {
		logrus.WithField("path", path).Debug("Path not handled. This is a bug, please open an issue against the kubernetes/test-infra repository with this error message.")
		return unmatchedPath
	}
	return resolvedPath
}

type Node struct {
	PathFragment
	children []Node
	// Greedy makes the node match all remnaining path elements as well
	Greedy bool
}

// PathFragment Interface for tree leafs to help resolve paths
type PathFragment interface {
	Matches(part string) bool
	Represent() string
}

type literal string

func (l literal) Matches(part string) bool {
	return string(l) == part
}

func (l literal) Represent() string {
	return string(l)
}

type variable string

func (v variable) Matches(part string) bool {
	return true
}

func (v variable) Represent() string {
	return ":" + string(v)
}

func L(fragment string, children ...Node) Node {
	return Node{
		PathFragment: literal(fragment),
		children:     children,
	}
}

func VGreedy(fragment string) Node {
	return Node{
		PathFragment: variable(fragment),
		Greedy:       true,
	}
}

func V(fragment string, children ...Node) Node {
	return Node{
		PathFragment: variable(fragment),
		children:     children,
	}
}

func resolve(parent Node, path []string) (string, bool) {
	if !parent.Matches(path[0]) {
		return "", false
	}
	representation := parent.Represent()
	if len(path) == 1 || parent.Greedy {
		return representation, true
	}
	for _, child := range parent.children {
		suffix, matched := resolve(child, path[1:])
		if matched {
			return strings.Join([]string{representation, suffix}, "/"), true
		}
	}
	return "", false
}
