package test

const (
	ProfileName      = "cov-profile.txt"
	StdoutName       = "stdout.txt"
	CovTargetRootRel = "testTarget"
	CovTargetRelPath = CovTargetRootRel + "/presubmit"
)

var (
	tmpArtsDir        = AbsPath("test_output/tmp_artifacts")
	InputArtifactsDir = AbsPath("testdata/artifacts")
	CovTargetDir      = AbsPath(CovTargetRelPath) + "/"
)
