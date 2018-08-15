// package artsTest stores artifacts functions for tests,
// used by other packages
package artsTest

import (
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/test"
)

type LocalArtifacts = artifacts.LocalArtifacts

var NewArtifacts = artifacts.New

func LocalArtsForTest(dirPrefix string) *LocalArtifacts {
	return &LocalArtifacts{Artifacts: *NewArtifacts(
		test.NewArtsDir(dirPrefix),
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}

func LocalInputArtsForTest() *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.New(
		test.InputArtifactsDir,
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}

func LocalArtsForTest_KeyfileNotExist(dirPrefix string) *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.New(
		test.NewArtsDir(dirPrefix),
		test.ProfileName,
		"key-cov-profile-dne.txt",
		test.StdoutName,
	)}
}
