package artsTest

import (
	"github.com/kubernetes/test-infra/coverage/artifacts"
	"github.com/kubernetes/test-infra/coverage/test"
)

type LocalArtifacts = artifacts.LocalArtifacts
var NewArtifacts = artifacts.NewArtifacts


func LocalArtsForTest(dirPrefix string) *LocalArtifacts {
	return &LocalArtifacts{Artifacts: *NewArtifacts(
		test.NewArtsDir(dirPrefix),
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}

func LocalInputArtsForTest() *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.NewArtifacts(
		test.InputArtifactsDir,
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}

func LocalArtsForTest_KeyfileNotExist(dirPrefix string) *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.NewArtifacts(
		test.NewArtsDir(dirPrefix),
		test.ProfileName,
		"key-cov-profile-dne.txt",
		test.StdoutName,
	)}
}
