// Package artsTest stores artifacts functions for tests,
// used by other packages
package artsTest

import (
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/test"
)

// LocalArtsForTest creates a LocalArtifacts object with a new unique
// artifact directory (to store intermediate output and prevent race
// condition in file IO); other fields filled up with test values
func LocalArtsForTest(dirPrefix string) *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.New(
		test.NewArtsDir(dirPrefix),
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}

// LocalInputArtsForTest creates a LocalArtifacts object with an artifact dir
// directory that stores artifacts for tests; other fields filled up with test
// values
func LocalInputArtsForTest() *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.New(
		test.InputArtifactsDir,
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}


