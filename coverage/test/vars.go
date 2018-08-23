package test

const (
	ProfileName      = "cov-profile.txt"
	StdoutName       = "stdout.txt"
	CovTargetRootRel = "testTarget"
	CovTargetRelPath = CovTargetRootRel + "/presubmit"
)

var (
	tmpArtsDir        = absPath("test_output/tmp_artifacts")
	InputArtifactsDir = absPath("testdata/artifacts")
	CovTargetDir      = absPath(CovTargetRelPath) + "/"
)
