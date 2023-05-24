/*
 * Copyright 2020 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package labels contains helper functions for working with labels.
package labels

import (
	"bytes"
	"path"
	"strings"
)

// Label represents a Bazel target label.
type Label struct {
	Repository string // Repository of the target, can be empty if the target belongs to the current repository
	Package    string // Package of a target, can be empty for top packages
	Target     string // Name of the target, should be always non-empty
}

// Format returns a string representation of a label. It's always absolute but
// the target name is omitted if it's equal to the package directory, e.g.
// "//package/foo:foo" is formatted as "//package/foo".
func (l Label) Format() string {
	b := new(bytes.Buffer)
	if l.Repository != "" {
		b.WriteString("@")
		b.WriteString(l.Repository)
	}
	if l.Repository == l.Target && l.Package == "" {
		return b.String()
	}
	b.WriteString("//")
	b.WriteString(l.Package)
	if l.Target != path.Base(l.Package) {
		b.WriteString(":")
		b.WriteString(l.Target)
	}
	return b.String()
}

// FormatRelative returns a string representation of a label relative to `pkg`
// (relative label if it represents a target in the same package, absolute otherwise)
func (l Label) FormatRelative(pkg string) string {
	if l.Repository != "" || pkg != l.Package {
		// External repository or different package
		return l.Format()
	}
	return ":" + l.Target
}

// Parse parses an absolute Bazel label (eg. //devtools/buildozer:rule)
// and returns the corresponding Label object.
func Parse(target string) Label {
	label := Label{}
	if strings.HasPrefix(target, "@") {
		target = strings.TrimLeft(target, "@")
		parts := strings.SplitN(target, "/", 2)
		if len(parts) == 1 {
			// "@foo" -> @foo//:foo
			return Label{target, "", target}
		}
		label.Repository = parts[0]
		target = "/" + parts[1]
	}
	parts := strings.SplitN(target, ":", 2)
	parts[0] = strings.TrimPrefix(parts[0], "//")
	label.Package = parts[0]
	if len(parts) == 2 && parts[1] != "" {
		label.Target = parts[1]
	} else if !strings.HasPrefix(target, "//") {
		// Maybe not really a label, store everything in Target
		label.Target = target
		label.Package = ""
	} else {
		// "//absolute/pkg" -> "absolute/pkg", "pkg"
		label.Target = path.Base(parts[0])
	}
	return label
}

// ParseRelative parses a label `input` which may be absolute or relative.
// If it's relative then it's considered to belong to `pkg`
func ParseRelative(input, pkg string) Label {
	if !strings.HasPrefix(input, "@") && !strings.HasPrefix(input, "//") {
		return Label{Package: pkg, Target: strings.TrimLeft(input, ":")}
	}
	return Parse(input)
}

// Shorten rewrites labels to use the canonical form (the form
// recommended by build-style).
// "//foo/bar:bar" => "//foo/bar", or ":bar" if the label belongs to pkg
func Shorten(input, pkg string) string {
	if !strings.HasPrefix(input, "//") && !strings.HasPrefix(input, "@") {
		// It doesn't look like a long label, so we preserve it.
		// Maybe it's not a label at all, e.g. a filename.
		return input
	}
	label := Parse(input)
	return label.FormatRelative(pkg)
}

// Equal returns true if label1 and label2 are equal. The function
// takes care of the optional ":" prefix and differences between long-form
// labels and local labels (relative to pkg).
func Equal(label1, label2, pkg string) bool {
	return ParseRelative(label1, pkg) == ParseRelative(label2, pkg)
}
