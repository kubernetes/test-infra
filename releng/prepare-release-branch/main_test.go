/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	args := []string{
		"prepare-release-branch",
		"/bin/config-rotator",
		"/bin/config-forker",
	}

	opts, err := parseArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.rotatorBin != "/bin/config-rotator" {
		t.Errorf("rotatorBin = %q, want /bin/config-rotator", opts.rotatorBin)
	}

	if opts.forkerBin != "/bin/config-forker" {
		t.Errorf("forkerBin = %q, want /bin/config-forker", opts.forkerBin)
	}
}

func TestParseArgsTooFew(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"prepare-release-branch", "/bin/rotator"})
	if err == nil {
		t.Fatal("expected error for too few args")
	}
}

func TestParseArgsTooMany(t *testing.T) {
	t.Parallel()

	args := []string{
		"prepare-release-branch",
		"/bin/rotator",
		"/bin/forker",
		"extra",
	}

	_, err := parseArgs(args)
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestParseArgsEmpty(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}
